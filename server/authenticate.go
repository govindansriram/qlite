/*
Author: Sriram Govindan
Date: 2024-07-17
Description: all functions used in the authorization process

Authentication Process

1) Initiate the tcp/ip handshake
2) The qlite server will send a greeting packet containing a challenge
    - the packet will be structured with the following byte structure
    - the first bytes contain the size of the message proceeding
    - the next 10 bytes will contain the ascii encoded word 'greetings'
    - this will be followed by a semicolon
    - the following 16 bytes will be an uuid4 string which is the challenge
3) The client response will consist of three parts
    - the first 4 bytes contains the length of the message
    - the next byte should be an uint8 number this signifies the role with 0 as publisher and 1 as subscriber
    - next the username should be encoded as ascii characters only using alphanumerics, this can take a
    maximum of 12 bytes. To signify the end of the username use a semicolon as a deliminator.
    - finally the password + challenge should be encrypted using the bcrypt algorithm for 10 rounds
4) if we are able to maintain a connection expect to receive message with the following structure
   - first 4 bytes will hold the content length of the following message
   - following bytes will contain the ascii encoded message pass
5) Otherwise, expect to receive an error message structured as such
   - first 4 bytes will hold the content length of the following message
   - ascii encoded error string
   - The connection will also automatically close
*/

package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

/*
generateChallenge

	creates a challenge packet. The challenge packet contains the string `greetings;`
	followed by an uuid. The uuid represents the challenge the password must be encoded
	with.
*/
func generateChallenge() ([26]byte, uuid.UUID) {
	challenge, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}

	var greetingPacket [26]byte

	copy(greetingPacket[:len("greetings;")], "greetings;")
	copy(greetingPacket[len("greetings;"):], challenge[:])

	return greetingPacket, challenge
}

/*
parseCredentials

	in response to a challenge packet a user will send there role, username, and encrypted password, this
	function extracts them all
*/
func parseCredentials(message []byte) (err error, role string, username []byte, password []byte) {

	if len(message) == 0 {
		err = errors.New("error: message is empty")
		return
	}

	roleNumber := message[0]

	if roleNumber == 0 {
		role = "publisher"
	} else {
		role = "subscriber"
	}

	splitArr := bytes.Split(message[1:], []byte(";"))

	if len(splitArr) != 2 {
		err = errors.New("error: authentication packet formatted incorrectly")
		return
	}

	username = splitArr[0]
	password = splitArr[1]

	return
}

/*
getUser

checks if a server contains a user with the specified role
*/
func getUser(serverUsers []User, username []byte, role string) (*User, error) {

	var pUser *User
	for _, user := range serverUsers {
		if bytes.Equal(user.name, username) {
			pUser = &user
		}
	}

	if pUser == nil {
		return nil, errors.New("error: user is not registered for this server")
	}

	if role == "subscriber" && !pUser.subscriber {
		return nil, errors.New("error: user does not have the subscriber role")
	}

	if role == "publisher" && !pUser.publisher {
		return nil, errors.New("error: user does not have the publisher role")
	}

	return pUser, nil
}

/*
checkPassword

ensures an encrypted password matches the raw password
*/
func checkPassword(password []byte, challenge [16]byte, provided []byte) bool {
	validPassword := make([]byte, len(password)+len(challenge))

	copy(validPassword, password)
	copy(validPassword[len(password):], challenge[:])

	err := bcrypt.CompareHashAndPassword(provided, validPassword)

	if err != nil {
		return false
	}

	return true
}

func checkSpace(
	role string,
	pubCount *atomic.Int32,
	maxPubCount uint16,
	subCount *atomic.Int32,
	maxSubCount uint16,
	lock *sync.Mutex) (*atomic.Int32, error) {

	var pCounter *atomic.Int32
	var maxUsers uint16

	if role == "publisher" {
		pCounter = pubCount
		maxUsers = maxPubCount
	} else if role == "subscriber" {
		pCounter = subCount
		maxUsers = maxSubCount
	} else {
		panic(fmt.Sprintf("invalid role provided: %s", role))
	}

	lock.Lock()
	defer lock.Unlock()
	currentCount := uint16(pCounter.Load())

	if currentCount >= maxUsers {
		return nil, fmt.Errorf("error: the max amount of %ss have already been connected", role)
	}

	pCounter.Add(1)
	return pCounter, nil
}

type reqState struct {
	isAlive bool
	role    string
	counter *atomic.Int32
}

/*
authenticate

checks if a connection has valid credentials to maintain a connection
*/
func authenticate(serv *Server, conn net.Conn) (alive bool, role string, counter *atomic.Int32) {

	response := make(chan reqState)

	go func() {
		challengePacket, challengeUuid := generateChallenge()
		state := writeMessage(conn, challengePacket[:], serv.maxIoSeconds)

		if !state {
			response <- reqState{}
			return
		}

		message, state := readMessage(conn, serv.maxIoSeconds)

		if !state {
			response <- reqState{}
			return
		}

		err, userRole, username, passwordAndChallenge := parseCredentials(message)

		if err != nil {
			writeCriticalError(conn, err, serv.maxIoSeconds)
			response <- reqState{}
			return
		}

		pUser, err := getUser(serv.users, username, userRole)

		if err != nil {
			writeCriticalError(conn, err, serv.maxIoSeconds)
			response <- reqState{}
			return
		}

		isValid := checkPassword(pUser.password, challengeUuid, passwordAndChallenge)

		if !isValid {
			err = errors.New("error: password is invalid")
			writeCriticalError(conn, err, serv.maxIoSeconds)
			response <- reqState{}
			return
		}

		pCounter, err := checkSpace(
			userRole,
			&serv.currentPublisher,
			serv.maxPublisherConnections,
			&serv.currentSubscribers,
			serv.maxSubscriberConnections,
			&serv.lock)

		if err != nil {
			writeCriticalError(conn, err, serv.maxIoSeconds)
			response <- reqState{}
			return
		}

		state = writeMessage(conn, []byte("PASS;"), serv.maxIoSeconds)

		if !state {
			response <- reqState{}
			return
		}

		response <- reqState{
			true,
			userRole,
			pCounter,
		}
	}()

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(serv.maxIoSeconds*3))
	defer cancel()

	select {
	case <-ctx.Done():
		return false, "", nil
	case resp := <-response:
		return resp.isAlive, resp.role, resp.counter
	}
}
