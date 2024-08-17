package server

import (
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

var verbose bool

func changeVerbosity() {
	verbose = !verbose
}

func logMessage(statement string, extra ...string) {
	if !verbose {
		return
	}

	statement = fmt.Sprintf(statement, extra)
	log.Println(statement)
}

/*
uniqueConnections

holds all currently active connections, useful for closing all active sessions in case of a shutdown
*/

type uniqueConnections struct {
	currentId atomic.Uint32
	connMap   sync.Map
}

/*
write

stores a connection
*/
func (u *uniqueConnections) write(conn net.Conn) uint32 {
	key := u.currentId.Add(1)
	u.connMap.Store(key, conn)
	return key
}

/*
remove

deletes a connection
*/
func (u *uniqueConnections) remove(key uint32) {
	val, loaded := u.connMap.LoadAndDelete(key)
	if !loaded {
		return
	}

	con := val.(net.Conn)
	closeConn(con)
	return
}

type User struct {
	name       []byte // the username
	password   []byte // the users password
	publisher  bool   // user can write messages to the qu
	subscriber bool   // user can read messages from the qu
}

/*
NewUser

create a user object
*/
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
	address                  string        // the ip address of the server
	port                     uint16        // the port the server will run on
	maxSubscriberConnections uint16        // the max amount of subscriber connections
	maxPublisherConnections  uint16        // the max amount of publisher connections
	maxMessages              uint32        // the max amount of messages that can be in the qu
	maxMessageSize           uint32        // the max size a message can be
	maxIoSeconds             time.Duration // how much time can be spent waiting for IO messages to complete
	pollingTimeSeconds       time.Duration // how long to poll the queue for a response
	maxHiddenTime            time.Duration // how long the message can be hidden from consumers for
	lock                     sync.Mutex    // synchronizes access to the server settings
	currentPublisher         atomic.Int32  // the current amount of publishers connected
	currentSubscribers       atomic.Int32  // the current amount of subscribers connected
	stopChannel              chan struct{} // open closing the server is stopped
}

func NewServer(
	users []User,
	port,
	maxSubs,
	maxPubs uint16,
	maxMess uint32,
	maxMessSize uint32,
	maxIoTimeSeconds uint16,
	maxHiddenTime uint16,
	maxPollingTimeSeconds uint16,
	address string,
	verbose bool) (*Server, error) {

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

	if address == "" {
		address = "localhost"
	}

	if maxHiddenTime == 0 {
		maxHiddenTime = 30
	}

	if verbose {
		changeVerbosity()
	}

	return &Server{
		users:                    users,
		port:                     port,
		maxSubscriberConnections: maxSubs,
		maxPublisherConnections:  maxPubs,
		maxMessages:              maxMess,
		maxHiddenTime:            time.Second * time.Duration(maxHiddenTime),
		maxIoSeconds:             time.Duration(maxIoTimeSeconds) * time.Second,
		pollingTimeSeconds:       time.Duration(maxPollingTimeSeconds) * time.Second,
		maxMessageSize:           maxMessSize,
		stopChannel:              make(chan struct{}),
	}, nil
}

func (s *Server) Start() {
	server := fmt.Sprintf("%s:%d", s.address, s.port)
	tcpAddr, err := net.ResolveTCPAddr("tcp", server)

	if err != nil {
		logMessage("Failed to resolve TCP address: %v", err.Error())
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)

	if err != nil {
		logMessage("could not start server due to: %v", err.Error())
		return
	}

	defer func() {
		err := listener.Close()
		if err != nil {
			logMessage(err.Error())
		}
	}()

	logMessage("server running on %s \n", server)

	listenerLoop(s.stopChannel, s, listener)
}

func (s *Server) Stop() {
	close(s.stopChannel)
}

func listenerLoop(
	finished <-chan struct{},
	server *Server,
	listener *net.TCPListener,
) {
	connections := uniqueConnections{}
	defer func() {
		connections.connMap.Range(func(key, value any) bool {
			pid := key.(uint32)
			connections.remove(pid)
			return true
		})
	}()

	workers := make(chan struct{}, server.maxPublisherConnections+server.maxSubscriberConnections)
	q := NewMSQueue(server.maxMessageSize, server.maxHiddenTime)

	for {
		select {
		case <-finished:
			q.Kill()
			return
		case workers <- struct{}{}:
			select {
			case <-finished:
				q.Kill()
				return
			default:
			}

			err := listener.SetDeadline(time.Now().Add(5 * time.Second))

			if err != nil {
				q.Kill()
				logMessage(err.Error())
				return
			}

			conn, err := listener.Accept()

			if err != nil {
				<-workers
				logMessage("experienced a error trying to establish a connection: %v", err.Error())
				continue
			}

			pid := connections.write(conn)

			go func() {
				state, role, pCounter := authenticate(server, conn) // why is pCounter used
				defer func() {
					closeConn(conn)
					connections.remove(pid)
					<-workers
				}()

				if !state {
					return
				}

				defer func() {
					pCounter.Add(-1)
				}()

				receiveRequests(conn, server, q, role)
			}()
		}
	}
}
