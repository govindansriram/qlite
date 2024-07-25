package server

import (
	"benchai/qlite/queue"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

type uniqueConnections struct {
	currentId atomic.Uint32
	lock      sync.Mutex
	connMap   map[uint32]net.Conn
}

func (u *uniqueConnections) write(conn net.Conn) uint32 {
	u.lock.Lock()
	defer u.lock.Unlock()

	key := u.currentId.Load()

	u.connMap[key] = conn
	u.currentId.Add(1)

	return key
}

func (u *uniqueConnections) remove(key uint32) {
	u.lock.Lock()
	defer u.lock.Unlock()
	delete(u.connMap, key)
}

func create(expectedSize uint16) uniqueConnections {
	return uniqueConnections{
		connMap: make(map[uint32]net.Conn, expectedSize),
	}
}

type User struct {
	name       []byte // the username
	password   []byte // the users password
	publisher  bool   // user can write messages to the queue
	subscriber bool   // user can read messages from the queue
}

func NewUser(name, password string, publisher, subscriber bool) (*User, error) {

	if len(name) == 0 {
		return nil, errors.New("name is empty")
	}

	if len(name) > 12 {
		return nil, fmt.Errorf("username %s is over the 12 byte limit", name)
	}

	if len(password) == 0 {
		return nil, errors.New("password is empty")
	}

	if !publisher && !subscriber {
		return nil, errors.New("user is neither a publisher or subscriber")
	}

	return &User{
		[]byte(name),
		[]byte(password),
		publisher,
		subscriber,
	}, nil
}

type Server struct {
	users                    []User        // the users that can establish connections
	port                     uint16        // the port the server will run on
	maxSubscriberConnections uint16        // the max amount of subscriber connections
	maxPublisherConnections  uint16        // the max amount of publisher connections
	maxMessages              uint32        // the max amount of messages that can be in the queue
	maxMessageSize           uint32        // the max size a message can be
	maxIoSeconds             time.Duration // how much time can be spent waiting for IO messages to complete
	pollingTimeSeconds       time.Duration // how long to poll the queue for a response
}

func NewServer(
	users []User,
	port,
	maxSubs,
	maxPubs uint16,
	maxMess uint32,
	maxMessSize uint32,
	maxIoTimeSeconds uint16,
	maxPollingTimeSeconds uint16) (*Server, error) {

	if len(users) == 0 {
		return nil, errors.New("no users are present for connection")
	}

	if port == 0 {
		port = 8080
	}

	if port > 49151 || port < 1024 {
		return nil, fmt.Errorf("port %d is not valid, it is not in the range 1024 - 49151", port)
	}

	if maxSubs == 0 {
		maxSubs = 2
	}

	if maxPubs == 0 {
		maxPubs = 1
	}

	if maxMess == 0 {
		maxMess = 100
	}

	if maxMessSize == 0 {
		maxMessSize = 9 * MB
	}

	if maxIoTimeSeconds == 0 {
		maxIoTimeSeconds = 3
	}

	if maxPollingTimeSeconds == 0 {
		maxPollingTimeSeconds = 10
	}

	return &Server{
		users:                    users,
		port:                     port,
		maxSubscriberConnections: maxSubs,
		maxPublisherConnections:  maxPubs,
		maxMessages:              maxMess,
		maxIoSeconds:             time.Duration(maxIoTimeSeconds) * time.Second,
		pollingTimeSeconds:       time.Duration(maxPollingTimeSeconds) * time.Second,
		maxMessageSize:           maxMessSize,
	}, nil
}

func (s Server) Start(kill <-chan struct{}) {
	server := fmt.Sprintf("localhost:%d", s.port)
	listener, err := net.Listen("tcp", server)

	if err != nil {
		log.Printf("could not start server due to: %v", err)
		return
	}

	defer func() {
		err := listener.Close()
		if err != nil {
			log.Println(err)
		}
	}()

	log.Printf("server running on %s \n", server)

	finished := make(chan struct{})

	go func() {
		<-kill
		finished <- struct{}{}
	}()

	listenerLoop(finished, s, listener)
}

func listenerLoop(
	finished <-chan struct{},
	server Server,
	listener net.Listener,
) {
	connections := create(server.maxPublisherConnections + server.maxSubscriberConnections)
	defer func() {
		for _, con := range connections.connMap {
			closeConn(con)
		}
	}()

	workers := make(chan struct{}, server.maxPublisherConnections+server.maxSubscriberConnections)
	q := queue.NewQueue(server.maxMessages, server.maxMessageSize)
	var publisherCount atomic.Int32
	var subscriberCount atomic.Int32
	lock := sync.Mutex{}

	for {
		select {
		case <-finished:
			return
		case workers <- struct{}{}:
			go func() {
				conn, err := listener.Accept()

				if err != nil {
					closeConn(conn)
					<-workers
					log.Printf("experienced a error trying to establish a connection: %v \n", err)
				}

				pid := connections.write(conn)

				state, pUser, role := authenticate(server, conn)
				defer func() {
					closeConn(conn)
					connections.remove(pid)
					<-workers
				}()

				if !state {
					return
				}

				var pCounter *atomic.Int32
				var maxConn uint16

				if role == "publisher" {
					pCounter = &publisherCount
					maxConn = server.maxPublisherConnections
				} else if role == "subscriber" {
					pCounter = &subscriberCount
					maxConn = server.maxSubscriberConnections
				}

				lock.Lock()

				if uint16(pCounter.Load()) < maxConn {
					pCounter.Add(1)
					lock.Unlock()
				} else {
					lock.Unlock()
					connectionFull(*pUser, conn, server.maxIoSeconds)
					return
				}

				defer func() {
					pCounter.Add(-1)
				}()

				receiveRequests(conn, server, &q, role)
			}()
		}
	}
}
