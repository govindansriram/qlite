package server

import (
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
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
	maxMessages              uint16        // the max amount of messages that can be in the queue
	maxIoSeconds             time.Duration // how much time can be spent waiting for IO messages to complete
}

func NewServer(users []User, port, maxSubs, maxPubs, maxMess uint16) (*Server, error) {
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
		maxSubs = 1
	}

	if maxPubs == 0 {
		maxPubs = 1
	}

	if maxMess == 0 {
		maxMess = 100
	}

	return &Server{
		users:                    users,
		port:                     port,
		maxSubscriberConnections: maxSubs,
		maxPublisherConnections:  maxPubs,
		maxMessages:              maxMess,
		maxIoSeconds:             time.Second * 2,
	}, nil
}

func (s Server) Start(kill <-chan struct{}) {
	server := fmt.Sprintf("localhost:%d", s.port)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	connections := create(s.maxPublisherConnections + s.maxSubscriberConnections)
	listener, err := net.Listen("tcp", server)

	if err != nil {
		log.Fatalf("could not start server due to: %v", err)
	}

	log.Printf("server running on %s \n", server)

	finished := make(chan struct{})

	go func() {
		<-sigChan
		gracefulShutdown(finished, &connections, listener)
		os.Exit(0)
	}()

	go func() {
		<-kill
		gracefulShutdown(finished, &connections, listener)
	}()

	listenerLoop(finished, s, listener, &connections)
}

func listenerLoop(
	finished <-chan struct{},
	server Server,
	listener net.Listener,
	connections *uniqueConnections,
) {

	workers := make(chan struct{}, server.maxPublisherConnections+server.maxSubscriberConnections)

	var publisherCount atomic.Int32
	var subscriberCount atomic.Int32

	lock := sync.Mutex{}

	for {
		select {
		case <-finished:
			return
		default:
			workers <- struct{}{}

			conn, err := listener.Accept()

			if err != nil {
				closeConn(conn)
				<-workers
				log.Printf("experienced a error trying to establish a connection: %v \n", err)
				continue
			}

			pid := connections.write(conn)

			go func() {

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

				if role == "publisher" {
					pCounter = &publisherCount
				} else if role == "subscriber" {
					pCounter = &subscriberCount
				}

				lock.Lock()

				cond1 := uint16(publisherCount.Load()) < server.maxPublisherConnections
				cond2 := uint16(subscriberCount.Load()) < server.maxSubscriberConnections

				if cond1 || cond2 {
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

			}()
		}
	}
}
