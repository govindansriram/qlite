package server

import (
	"bytes"
	"errors"
	"fmt"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net"
	"sync"
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
	conns := uniqueConnections{}

	wg := sync.WaitGroup{}

	for range routines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = conns.write(testConnection("test"))
		}()
	}

	wg.Wait()

	count := 0

	conns.connMap.Range(func(key, value any) bool {
		count++
		return true
	})

	if count != routines {
		t.Error("writes failed")
	}
}

func TestUniqueConnections_remove(t *testing.T) {
	routines := 30
	conns := uniqueConnections{}

	for range routines {
		_ = conns.write(testConnection("test"))
	}

	wg := sync.WaitGroup{}

	for idx := range routines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conns.remove(uint32(idx + 1))
		}()
	}

	wg.Wait()
	count := 0

	conns.connMap.Range(func(key, value any) bool {
		fmt.Println(key, value)
		count++
		return true
	})

	if count != 0 {
		t.Error("reads failed")
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

func getListener() (*net.TCPListener, error) {
	const network uint16 = 8080

	server := fmt.Sprintf("%s:%d", "localhost", network)
	tcpAddr, err := net.ResolveTCPAddr("tcp", server)

	if err != nil {
		return nil, err
	}

	listener, err := net.ListenTCP("tcp", tcpAddr)

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
			1000000,
			60,
			5,
			10,
			"",
			true)

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
