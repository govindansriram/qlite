package server

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"math/rand/v2"
	"net"
	"testing"
	"time"
)

func generateStorageMessage() []byte {
	dataCount := uint32(100)
	dataList := make([]byte, 0, 4*dataCount)

	for i := range dataCount {
		intSlice := make([]byte, 4)
		binary.LittleEndian.PutUint32(intSlice, i)
		dataList = append(dataList, intSlice...)
	}

	return dataList
}

func push(clientConn net.Conn, deadline time.Duration) (bool, error) {
	data := generateStorageMessage()
	prefix := []byte("PUSH;")

	mess := make([]byte, len(data)+len(prefix))
	copy(mess, prefix)
	copy(mess[len(prefix):], data)

	alive := writeMessage(clientConn, mess, deadline)

	if !alive {
		return false, nil
	}

	mess, alive = readMessage(clientConn, deadline)

	if !alive {
		return false, nil
	}

	before, after, found := bytes.Cut(mess, []byte(";"))

	if !found {
		return false, nil
	}

	if bytes.Equal(before, []byte("FAIL")) {
		fmt.Println("error", errors.New(string(after)))
		return true, errors.New(string(after))
	}

	return true, nil
}

func deleteMessage(clientConn net.Conn, deadline time.Duration, uid uuid.UUID) (bool, error) {
	prefix := []byte("DEL;")
	mess := make([]byte, len(prefix)+16)
	copy(mess, prefix)
	copy(mess[len(prefix):], uid[:])

	alive := writeMessage(clientConn, mess, deadline)
	if !alive {
		return false, nil
	}

	mess, alive = readMessage(clientConn, deadline)

	if !alive {
		return false, nil
	}

	before, after, found := bytes.Cut(mess, []byte(";"))

	if !found {
		return false, nil
	}

	if bytes.Equal(before, []byte("FAIL")) {
		return true, errors.New(string(after))
	}

	return true, nil
}

func hide(clientConn net.Conn, deadline time.Duration) (bool, error) {
	prefix := []byte("HIDE;")

	lifetime := make([]byte, 4)
	binary.LittleEndian.PutUint32(lifetime, 3)

	mess := make([]byte, len(prefix)+len(lifetime))
	copy(mess, prefix)
	copy(mess[len(prefix):], lifetime)

	alive := writeMessage(clientConn, mess, deadline)
	if !alive {
		return false, nil
	}

	mess, alive = readMessage(clientConn, deadline)

	if !alive {
		return false, nil
	}

	before, after, found := bytes.Cut(mess, []byte(";"))

	if !found {
		return false, nil
	}

	if bytes.Equal(before, []byte("FAIL")) {
		return true, errors.New(string(after))
	}

	uid, err := uuid.FromBytes(after[:16])
	if err != nil {
		panic(err)
	}

	randTime := rand.IntN(5001)
	time.Sleep(time.Duration(randTime) * time.Millisecond)

	return deleteMessage(clientConn, deadline, uid)
}

func poll(clientConn net.Conn, deadline time.Duration) (bool, error) {
	prefix := []byte("POLL;")

	lifetime := make([]byte, 4)
	binary.LittleEndian.PutUint32(lifetime, 3)

	mess := make([]byte, len(prefix)+len(lifetime))
	copy(mess, prefix)
	copy(mess[len(prefix):], lifetime)

	alive := writeMessage(clientConn, mess, deadline)
	if !alive {
		return false, nil
	}

	mess, alive = readMessage(clientConn, deadline)
	if !alive {
		return false, nil
	}

	before, after, found := bytes.Cut(mess, []byte(";"))

	if !found {
		return false, nil
	}

	if bytes.Equal(before, []byte("FAIL")) {
		return true, errors.New(string(after))
	}

	uid, err := uuid.FromBytes(after[:16])
	if err != nil {
		panic(err)
	}

	randTime := rand.IntN(5001)
	time.Sleep(time.Duration(randTime) * time.Millisecond)

	return deleteMessage(clientConn, deadline, uid)
}

func auth(user User, role string, conn net.Conn, deadline time.Duration) (bool, error) {
	mess, alive := readMessage(conn, deadline)

	if !alive {
		return false, errors.New("connection was closed unexpectedly")
	}

	_, after, found := bytes.Cut(mess, []byte(";"))

	if !found {
		_ = conn.Close()
		return false, errors.New("improper message was sent")
	}

	pAndC := make([]byte, len(user.password)+len(after))

	copy(pAndC, user.password)
	copy(pAndC[len(user.password):], after)

	pass, err := bcrypt.GenerateFromPassword(pAndC, bcrypt.DefaultCost)

	if err != nil {
		_ = conn.Close()
		return false, errors.New("failed to encrypt password")
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

	alive = writeMessage(conn, response, deadline)

	if !alive {
		return false, errors.New("connection was closed on write")
	}

	mess, alive = readMessage(conn, deadline)

	if !alive {
		return false, errors.New("connection was closed on read")
	}

	data := bytes.Split(mess, []byte(";"))

	if len(data) == 0 {
		_ = conn.Close()
		return false, errors.New("invalid response received")
	}

	if bytes.Equal(data[0], []byte("FAIL")) {
		if len(data) == 2 {
			return true, errors.New(string(data[1]))
		} else {
			return true, errors.New("could not parse message")
		}
	}

	return true, nil
}

func makeRequests(
	clientConn net.Conn,
	lifeline chan struct{},
	isPub bool,
	pubUser,
	subUser User,
	deadline time.Duration) {

	role := "publisher"
	usr := pubUser

	if !isPub {
		usr = subUser
		role = "subscriber"
	}

	state, err := auth(usr, role, clientConn, deadline)

	if !state {
		return
	}

	if err != nil {
		return
	}

	if isPub {
		fmt.Println("publisher connected")
		for {
			select {
			case <-lifeline:
				return
			default:
				randTime := rand.IntN(5001)
				time.Sleep(time.Duration(randTime) * time.Millisecond)
				state, err = push(clientConn, deadline)

				if state {
					fmt.Println("pushed")
				}

				if err != nil {
					fmt.Println("experienced publisher error", err)
				}

				if !state {
					fmt.Println("publisher connection closed")
					return
				}
			}
		}
	} else {
		fmt.Println("subscriber connected")
		count := 0
		for {
			select {
			case <-lifeline:
				return
			default:
				randTime := rand.IntN(5001)
				time.Sleep(time.Duration(randTime) * time.Millisecond)

				var state bool
				if count%2 == 0 {
					state, err = hide(clientConn, deadline)
				} else {
					state, err = poll(clientConn, deadline)
				}

				if state {
					fmt.Println("polled message")
				}

				if err != nil {
					fmt.Println("experienced subscriber error:", err)
				}

				if !state {
					fmt.Println("subscriber connection closed")
					return
				}

				count++
			}
		}
	}
}

func closeReqs(con net.Conn) {
	randTime := rand.IntN(20_000) + 10_000
	time.Sleep(time.Duration(randTime) * time.Millisecond)
	fmt.Println("deadline exceeded closing connection")
	_ = con.Close()
}

func handler(lifeline chan struct{}, deadline time.Duration, pubUser, subUser User) {
	connections := make([]net.Conn, 0, 100)
	point := 0

	for {
		isPub := point%2 == 0

		select {
		case <-lifeline:
			fmt.Println("terminating")
			for _, c := range connections {
				_ = c.Close()
			}
			return
		default:
			address := "localhost:8080"
			timeout := 1 * time.Second

			conn, err := net.DialTimeout("tcp", address, timeout)

			if err == nil {
				connections = append(connections, conn)
				go makeRequests(conn, lifeline, isPub, pubUser, subUser, deadline)
				go closeReqs(conn)
			}
		}

		point++
	}
}

func TestServer_Start(t *testing.T) {

	kChan := make(chan struct{})

	pub, err := NewUser("publisher", "password", true, false)

	if err != nil {
		panic(err)
	}

	sub, err := NewUser("subscriber", "password", false, true)

	if err != nil {
		panic(err)
	}
	users := make([]User, 2)
	users[1] = *sub
	users[0] = *pub

	serv, err := NewServer(
		users,
		8080,
		100,
		300,
		100000,
		100000,
		60,
		60,
		10,
		"",
		false)

	if err != nil {
		panic(err)
	}

	go func() {
		time.Sleep(30 * time.Second)
		close(kChan)
	}()

	go serv.Start(kChan)

	handler(kChan, serv.maxIoSeconds, *pub, *sub)
}
