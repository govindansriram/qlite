package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"log"
	"math/rand/v2"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type addr string

func (addr) Network() string {
	return ""
}

func (addr) String() string {
	return ""
}

type testConnection string

func (testConnection) Read(b []byte) (n int, err error) {
	return 0, nil
}

func (testConnection) Write(b []byte) (n int, err error) {
	return 0, nil
}

func (testConnection) Close() error {
	return nil
}

func (testConnection) LocalAddr() net.Addr {
	return addr("test")
}

func (testConnection) RemoteAddr() net.Addr {
	return addr("test")
}

func (testConnection) SetDeadline(t time.Time) error {
	return nil
}

func (testConnection) SetReadDeadline(t time.Time) error {
	return nil
}

func (testConnection) SetWriteDeadline(t time.Time) error {
	return nil
}

func TestUniqueConnections_write(t *testing.T) {
	routines := 30
	conns := newUniqueConnections(30)

	wg := sync.WaitGroup{}

	for range routines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = conns.write(testConnection("test"))
		}()
	}

	wg.Wait()

	if len(conns.connMap) != routines {
		t.Error("writes failed")
	}
}

func TestUniqueConnections_remove(t *testing.T) {
	routines := 30
	conns := newUniqueConnections(30)

	for range routines {
		_ = conns.write(testConnection("test"))
	}

	wg := sync.WaitGroup{}

	for idx := range routines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conns.remove(uint32(idx))
		}()
	}

	wg.Wait()

	if len(conns.connMap) != 0 {
		t.Error("removes failed")
	}
}

type testUser struct {
	name       string
	username   string
	password   string
	pub        bool
	sub        bool
	shouldPass bool
}

func (t testUser) isEq(other User) bool {
	if t.username != string(other.name) {
		return false
	}

	if t.password != string(other.password) {
		return false
	}

	if t.pub != other.publisher {
		return false
	}

	if t.sub != other.subscriber {
		return false
	}

	return true
}

var userTests = []testUser{
	{
		name:       "test valid user",
		username:   "test",
		password:   "test",
		pub:        true,
		sub:        true,
		shouldPass: true,
	},
	{
		name:     "test no username",
		username: "",
		password: "test",
		pub:      true,
		sub:      true,
	},
	{
		name:     "test no password",
		username: "user",
		password: "",
		pub:      true,
		sub:      true,
	},
	{
		name:     "test long username",
		username: "useruseruser",
		password: "",
		pub:      true,
		sub:      true,
	},
}

func TestNewUser(t *testing.T) {
	for _, userTest := range userTests {
		t.Run(userTest.name, func(t *testing.T) {
			pUSer, err := NewUser(
				userTest.username,
				userTest.password,
				userTest.pub,
				userTest.sub)

			if err != nil && userTest.shouldPass {
				t.Error("should not have errored")
				return
			}

			if err == nil && !userTest.shouldPass {
				t.Error("should have errored")
				return
			}

			if err == nil {
				if !userTest.isEq(*pUSer) {
					t.Error("values are not the same")
					return
				}
			}
		})
	}
}

func getListener() (net.Listener, error) {
	const network uint16 = 8080
	server := fmt.Sprintf("localhost:%d", network)
	listener, err := net.Listen("tcp", server)

	if err != nil {
		return nil, err
	}

	return listener, err
}

func testAuthenticate(
	clientConn net.Conn,
	maxIo time.Duration,
	user User,
	role string) error {
	mess, alive := readMessage(clientConn, maxIo)

	if !alive {
		return errors.New("connection was closed unexpectedly")
	}

	_, after, found := bytes.Cut(mess, []byte(";"))

	if !found {
		return errors.New("improper message was sent")
	}

	pAndC := make([]byte, len(user.password)+len(after))

	copy(pAndC, user.password)
	copy(pAndC[len(user.password):], after)

	pass, err := bcrypt.GenerateFromPassword(pAndC, bcrypt.DefaultCost)

	if err != nil {
		return errors.New("failed to encrypt password")
	}

	response := make([]byte, 2+len(user.name)+len(pass))

	if role == "publisher" {
		response[0] = 0
	} else {
		response[0] = 1
	}

	copy(response[1:], user.name)
	response[len(user.name)+1] = ';'
	copy(response[len(user.name)+2:], pass)

	alive = writeMessage(clientConn, response, maxIo)

	if !alive {
		return errors.New("connection was closed on write")
	}

	mess, alive = readMessage(clientConn, maxIo)

	if !alive {
		return errors.New("connection was closed on read")
	}

	data := bytes.Split(mess, []byte(";"))

	if len(data) == 0 {
		return errors.New("invalid response received")
	}

	if bytes.Equal(data[0], []byte("FAIL")) {
		if len(data) == 2 {
			return errors.New(string(data[1]))
		} else {
			return errors.New("could not parse message")
		}
	}

	return nil
}

func Test_listenerLoop(t *testing.T) {

	t.Run("standard test", func(t *testing.T) {
		list, err := getListener()

		if err != nil {
			t.Error(err)
		}

		defer func() {
			err = list.Close()
			log.Println(err)
		}()

		u1, err := NewUser("publisher", "test", true, false)

		if err != nil {
			t.Error(err)
		}

		u2, err := NewUser("subscriber", "test", false, true)

		if err != nil {
			t.Error(err)
		}

		users := []User{
			*u1, *u2,
		}

		server, err := NewServer(
			users,
			8080,
			5,
			3,
			100,
			10_000,
			5,
			10,
			"")

		if err != nil {
			t.Error(err)
		}

		kChan := make(chan struct{})

		defer func() {
			kChan <- struct{}{}
		}()

		go func() {
			listenerLoop(kChan, server, list)
		}()

		var conn net.Conn
		for range server.maxSubscriberConnections {
			network := fmt.Sprintf("localhost:%d", 8080)
			conn, err = net.Dial("tcp", network)

			if err == nil {
				break
			}
		}

		err = testAuthenticate(conn, server.maxIoSeconds, users[0], "publisher")

		if err != nil {
			t.Error(err)
			return
		}

		type conStruct struct {
			error error
			conn  net.Conn
		}

		connChan := make(chan conStruct)
		wg := sync.WaitGroup{}

		for range server.maxPublisherConnections {
			wg.Add(1)
			go func() {
				defer wg.Done()
				network := fmt.Sprintf("localhost:%d", 8080)
				conn, err = net.Dial("tcp", network)

				if err != nil {
					connChan <- conStruct{
						error: err,
					}
				} else {
					err := testAuthenticate(conn, server.maxIoSeconds, users[0], "publisher")
					connChan <- conStruct{
						error: err,
					}
				}
			}()
		}

		go func() {
			wg.Wait()
			close(connChan)
		}()

		var errCount uint8
		var lastError error

		for cn := range connChan {

			if cn.error != nil {
				errCount++
				lastError = cn.error
			}

			if errCount > 1 {
				t.Error(lastError)
				return
			}
		}
	})
}

func pushRandomData(conn net.Conn, maxIo time.Duration) (bool, error) {
	randTime := rand.IntN(6)
	time.Sleep(time.Millisecond * 1000 * time.Duration(randTime))

	byteSlice := make([]byte, (randTime+1)*100*2)

	for idx := range (randTime + 1) * 100 {
		byteSlice[idx] = 'a'
	}

	messageSlice := make([]byte, 5+len(byteSlice))

	copy(messageSlice, "PUSH;")
	copy(messageSlice[5:], byteSlice)

	alive := writeMessage(conn, messageSlice, maxIo)

	if !alive {
		return false, errors.New("connection expired unexpectedly")
	}

	mess, alive := readMessage(conn, maxIo)

	if !alive {
		return false, errors.New("connection expired unexpectedly")
	}

	bs := bytes.Split(mess, []byte(";"))

	if bytes.Equal(bs[0], []byte("PASS")) {
		return true, nil
	} else {
		return false, nil
	}
}

func popRandomData(conn net.Conn, maxIo time.Duration, poll time.Duration) (bool, error) {
	randTime := rand.IntN(6)
	time.Sleep(time.Millisecond * 1000 * time.Duration(randTime))

	isEven := (randTime % 2) == 0
	var mess []byte

	if isEven {
		mess = []byte("LPOP;")
		maxIo += poll
	} else {
		mess = []byte("SPOP;")
	}

	alive := writeMessage(conn, mess, maxIo)

	if !alive {
		return false, errors.New("connection expired unexpectedly")
	}

	mess, alive = readMessage(conn, maxIo)

	if !alive {
		return false, errors.New("connection expired unexpectedly")
	}

	bs := bytes.Split(mess, []byte(";"))

	if bytes.Equal(bs[0], []byte("PASS")) {
		return true, nil
	} else {
		return false, nil
	}
}

func subscriber(
	conn net.Conn,
	maxIo time.Duration,
	poll time.Duration,
	maxPops int) (int, error) {

	currentPops := 0
	resp := make(chan bool)
	errChan := make(chan error)
	total := int((maxIo * time.Duration(maxPops) * 2).Milliseconds())

	randTime := rand.IntN(total)

	ctx, cf := context.WithDeadline(
		context.Background(),
		time.Now().Add(time.Duration(randTime)*time.Millisecond))

	defer cf()

	for {
		if currentPops == maxPops {
			closeConn(conn)
			return maxPops, nil
		}
		go func() {
			state, err := popRandomData(conn, maxIo, poll)
			errChan <- err
			resp <- state

		}()

		select {
		case <-ctx.Done():
			closeConn(conn)

			<-errChan
			if <-resp {
				currentPops++
			}

			return currentPops, nil
		case e := <-errChan:
			if e == nil {
				if <-resp {
					currentPops++
				}
			} else {
				<-resp
				return 0, e
			}
		}
	}
}

func publisher(conn net.Conn, maxIo time.Duration) error {
	messages := 20

	for messages != 0 {
		popped, err := pushRandomData(conn, maxIo)

		if err != nil {
			return err
		}

		if popped {
			messages--
		}
	}

	closeConn(conn)
	return nil
}

func runSubscribers(
	maxSubscribers int,
	maxIo time.Duration,
	poll time.Duration,
	maxPops int,
	killCh <-chan struct{},
	errChan chan<- error,
	usr User,
	totalPops int) bool {

	maxSubs := make(chan struct{}, maxSubscribers)

	currentPops := atomic.Int32{}
	for {
		popped := currentPops.Load()
		if popped >= int32(totalPops) {
			return true
		}

		select {
		case <-killCh:
			return false
		case maxSubs <- struct{}{}:
			go func() {
				ctx, cf := context.WithDeadline(context.Background(), time.Now().Add(time.Second*3))
				var conn net.Conn
				var err error

				defer func() {
					cf()
					<-maxSubs
				}()

				stay := true
				for stay {
					select {
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					default:
						network := fmt.Sprintf("localhost:%d", 8080)
						conn, err = net.Dial("tcp", network)
						if err == nil {
							stay = false
						}
						err = testAuthenticate(conn, maxIo, usr, "subscriber")
						if err != nil {
							errChan <- err
						}
					}
				}

				popped, err := subscriber(conn, maxIo, poll, maxPops)
				if err != nil {
					errChan <- err
				}

				currentPops.Add(int32(popped))
			}()
		}
	}
}

func runPublishers(
	maxPublishers int,
	maxIo time.Duration,
	killCh <-chan struct{},
	errChan chan<- error,
	usr User) {
	maxPubs := make(chan struct{}, maxPublishers)

	alive := true
	for alive {
		select {
		case <-killCh:
			alive = false
		case maxPubs <- struct{}{}:
			go func() {
				ctx, cf := context.WithDeadline(context.Background(), time.Now().Add(time.Second*3))
				var conn net.Conn
				var err error

				defer func() {
					cf()
					<-maxPubs
				}()

				stay := true
				for stay {
					select {
					case <-ctx.Done():
						errChan <- ctx.Err()
						return
					default:
						network := fmt.Sprintf("localhost:%d", 8080)
						conn, err = net.Dial("tcp", network)
						if err == nil {
							stay = false
						}
						err = testAuthenticate(conn, maxIo, usr, "publisher")
						if err != nil {
							errChan <- err
						}
					}
				}

				err = publisher(conn, maxIo)
				if err != nil {
					errChan <- err
				}
			}()
		}
	}
}

func Test_Server(t *testing.T) {
	list, err := getListener()

	if err != nil {
		t.Error(err)
	}

	defer func() {
		err = list.Close()
		log.Println(err)
	}()

	u1, err := NewUser("publisher", "test", true, false)

	if err != nil {
		t.Error(err)
	}

	u2, err := NewUser("subscriber", "test", false, true)

	if err != nil {
		t.Error(err)
	}

	users := []User{
		*u1, *u2,
	}

	server, err := NewServer(
		users,
		8080,
		10,
		20,
		3000,
		10_000,
		5,
		10,
		"")

	if err != nil {
		t.Error(err)
		return
	}

	finishedListener := make(chan struct{})
	finishedPublisher := make(chan struct{})
	finishedSubscriber := make(chan struct{})
	lve := make(chan struct{})
	errChan := make(chan error)

	go func() {
		err := <-errChan
		t.Error(err)
		finishedPublisher <- struct{}{}
		finishedListener <- struct{}{}
		finishedSubscriber <- struct{}{}
	}()

	go func() {
		listenerLoop(finishedListener, server, list)
	}()

	ext := true
	var lenCon net.Conn

	for ext {
		network := fmt.Sprintf("localhost:%d", 8080)
		lenCon, err = net.Dial("tcp", network)
		if err == nil {
			ext = false
		}
	}

	err = testAuthenticate(lenCon, server.maxIoSeconds, users[0], "publisher")
	if err != nil {
		t.Fatal(err)
	}

	go func() {
		state := runSubscribers(
			int(server.maxSubscriberConnections),
			server.maxIoSeconds,
			server.pollingTimeSeconds,
			20,
			finishedSubscriber,
			errChan,
			users[1],
			400)

		if !state {
			t.Error("failed")
		}

		finishedPublisher <- struct{}{}
		finishedListener <- struct{}{}
		lve <- struct{}{}
	}()

	go func() {
		runPublishers(int(server.maxPublisherConnections-1), server.maxIoSeconds, finishedPublisher, errChan, users[0])
	}()

	var mess []byte
	for {
		select {
		case <-lve:
			return
		default:
			mess = []byte("LEN;")
			isAlive := writeMessage(lenCon, mess, server.maxIoSeconds)

			if !isAlive {
				t.Error("failed to get len")
				return
			}

			mess, isAlive = readMessage(lenCon, server.maxIoSeconds)

			if !isAlive {
				t.Error("failed to get len")
				return
			}

			_, _, found := bytes.Cut(mess, []byte(";"))

			if !found {
				t.Error("failed to get len")
				return
			}
		}
	}
}
