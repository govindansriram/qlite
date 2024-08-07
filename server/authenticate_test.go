package server

import (
	"bytes"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"testing"
	"time"
)

func Test_generateChallenge(t *testing.T) {
	t.Run("check packet value", func(t *testing.T) {
		greeting, uid := generateChallenge()

		validPacket := make([]byte, 0, len("greetings;")+16)
		validPacket = append(validPacket, []byte("greetings;")...)
		validPacket = append(validPacket, uid[:]...)

		if !bytes.Equal(validPacket, greeting[:]) {
			t.Error("improper challenge packet generated")
		}
	})
}

func Test_parseCredentials(t *testing.T) {
	type test struct {
		name     string
		role     string
		username []byte
		password []byte
		message  []byte
		isErr    bool
	}

	greeting := func(username, password []byte, isPublisher bool) []byte {

		mess := make([]byte, 0, len(username)+len(password)+2)

		if isPublisher {
			mess = append(mess, 0)
		} else {
			mess = append(mess, 1)
		}

		mess = append(mess, username...)
		mess = append(mess, ';')
		mess = append(mess, password...)

		return mess
	}

	tests := []test{
		{
			name:     "test standard subscriber message",
			role:     "subscriber",
			username: []byte("user"),
			password: []byte("test"),
			isErr:    false,
			message:  greeting([]byte("user"), []byte("test"), false),
		},
		{
			name:     "test standard publisher message",
			role:     "publisher",
			username: []byte("user"),
			password: []byte("test"),
			isErr:    false,
			message:  greeting([]byte("user"), []byte("test"), true),
		},
		{
			name:     "test missing semicolon",
			role:     "publisher",
			username: []byte("user"),
			password: []byte("test"),
			isErr:    true,
			message:  []byte("1walterwhite123"),
		},
		{
			name:     "test starting semicolon",
			role:     "publisher",
			username: []byte("walter"),
			password: []byte("white123"),
			isErr:    true,
			message:  []byte(";1walterwhite123"),
		},
		{
			name:     "test empty message",
			role:     "publisher",
			username: []byte(""),
			password: []byte(""),
			isErr:    true,
			message:  []byte(""),
		},
	}

	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			err, role, username, password := parseCredentials(tst.message)

			if err != nil && !tst.isErr {
				t.Errorf("received unexpected error: %v", err)
			}

			if err == nil && tst.isErr {
				t.Error("expected error received nil")
			}

			if err != nil && tst.isErr {
				return
			}

			if role != tst.role {
				t.Errorf("expected role %s received role %s", tst.role, role)
			}

			if !bytes.Equal(tst.username, username) {
				t.Errorf("expected username %s received username %s", tst.username, username)
			}

			if !bytes.Equal(tst.password, password) {
				t.Errorf("expected password %s received username %s", tst.password, password)
			}
		})
	}
}

func Test_getUser(t *testing.T) {

	users := []User{
		{
			name:      []byte("test1"),
			publisher: true,
		},
		{
			name:       []byte("test2"),
			subscriber: true,
		},
	}

	type test struct {
		name     string
		role     string
		username []byte
		isErr    bool
	}

	tests := []test{
		{
			name:     "test valid publisher",
			role:     "publisher",
			username: []byte("test1"),
			isErr:    false,
		},
		{
			name:     "test valid subscriber",
			role:     "subscriber",
			username: []byte("test2"),
			isErr:    false,
		},
		{
			name:     "test invalid username",
			role:     "publisher",
			username: []byte("test3"),
			isErr:    true,
		},
		{
			name:     "test invalid role",
			role:     "subscriber",
			username: []byte("test1"),
			isErr:    true,
		},
	}

	for _, tst := range tests {
		t.Run(tst.name, func(t *testing.T) {
			_, err := getUser(users, tst.username, tst.role)

			if err != nil && !tst.isErr {
				t.Errorf("received unexpected error: %v", err)
			}

			if err == nil && tst.isErr {
				t.Error("expected error received nil")
			}
		})
	}
}

func Test_checkPassword(t *testing.T) {

	t.Run("test matching passwords default cost", func(t *testing.T) {
		password := []byte("test password")
		challenge, err := uuid.NewUUID()

		if err != nil {
			t.Fatal(err)
		}

		challengeAndPassword := append(password, challenge[:]...)

		hashedPassword, err := bcrypt.GenerateFromPassword(challengeAndPassword, bcrypt.DefaultCost)
		if err != nil {
			t.Fatal(err)
		}

		if !checkPassword(password, challenge, hashedPassword) {
			t.Error("matching passwords declared as different")
		}

	})

	t.Run("test matching passwords small cost", func(t *testing.T) {
		password := []byte("test password")
		challenge, err := uuid.NewUUID()

		if err != nil {
			t.Fatal(err)
		}

		challengeAndPassword := append(password, challenge[:]...)

		hashedPassword, err := bcrypt.GenerateFromPassword(challengeAndPassword, 4)
		if err != nil {
			t.Fatal(err)
		}

		if !checkPassword(password, challenge, hashedPassword) {
			t.Error("matching passwords declared as different")
		}

	})

	t.Run("test different passwords default cost", func(t *testing.T) {
		password := []byte("test password")
		challenge, err := uuid.NewUUID()

		if err != nil {
			t.Fatal(err)
		}

		challengeAndPassword := append(password, challenge[:]...)

		hashedPassword, err := bcrypt.GenerateFromPassword(challengeAndPassword, 4)
		if err != nil {
			t.Fatal(err)
		}

		if checkPassword([]byte("fake password"), challenge, hashedPassword) {
			t.Error("different passwords declared as matching")
		}

	})

	t.Run("test different passwords default cost, different challenge", func(t *testing.T) {
		password := []byte("test password")
		challenge, err := uuid.NewUUID()

		if err != nil {
			t.Fatal(err)
		}

		challenge2, err := uuid.NewUUID()

		if err != nil {
			t.Fatal(err)
		}

		challengeAndPassword := append(password, challenge[:]...)

		hashedPassword, err := bcrypt.GenerateFromPassword(challengeAndPassword, 4)
		if err != nil {
			t.Fatal(err)
		}

		if checkPassword(password, challenge2, hashedPassword) {
			t.Error("different passwords declared as matching")
		}

	})

}

func Test_authenticate(t *testing.T) {

	users := []User{
		{
			name:       []byte("test"),
			password:   []byte("test123"),
			publisher:  true,
			subscriber: true,
		},
		{
			name:       []byte("test2222"),
			password:   []byte("test123"),
			subscriber: true,
		},
	}

	t.Run("test valid authentication", func(t *testing.T) {

		serv := Server{
			maxIoSeconds:             time.Second * 3,
			maxSubscriberConnections: 3,
			maxPublisherConnections:  3,
			users:                    users,
		}

		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
		}()

		reqChan := make(chan reqState)
		go func() {
			alive, role, counter := authenticate(&serv, serverConn)
			reqChan <- reqState{
				alive,
				role,
				counter,
			}
		}()

		mess, alive := readMessage(clientConn, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		splits := bytes.Split(mess, []byte(";"))

		if len(splits) != 2 {
			t.Errorf("improper message was sent")
			return
		}

		user := serv.users[0]

		pAndC := make([]byte, len(user.password)+len(splits[1]))

		copy(pAndC, user.password)
		copy(pAndC[len(user.password):], splits[1])

		pass, err := bcrypt.GenerateFromPassword(pAndC, bcrypt.DefaultCost)

		if err != nil {
			t.Errorf("failed to encrypt password")
			return
		}

		response := make([]byte, 2+len(user.name)+len(pass))

		response[0] = 0
		copy(response[1:], user.name)
		response[len(user.name)+1] = ';'
		copy(response[len(user.name)+2:], pass)

		alive = writeMessage(clientConn, response, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		mess, alive = readMessage(clientConn, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		serverState := <-reqChan

		if serverState.role != "publisher" {
			t.Error("authenticated the wrong role")
			return
		}

		if serverState.counter.Load() != 1 {
			t.Error("counter was not incremented")
			return
		}

		if !serverState.isAlive {
			t.Errorf("server connection closed unexpectedly")
		}
	})

	t.Run("test invalid username", func(t *testing.T) {

		serv := Server{
			maxIoSeconds:             time.Second * 3,
			maxSubscriberConnections: 3,
			maxPublisherConnections:  3,
			users:                    users,
		}

		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
		}()

		reqChan := make(chan reqState)
		go func() {
			alive, role, counter := authenticate(&serv, serverConn)
			reqChan <- reqState{
				alive,
				role,
				counter,
			}
		}()

		mess, alive := readMessage(clientConn, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		splits := bytes.Split(mess, []byte(";"))

		if len(splits) != 2 {
			t.Errorf("improper message was sent")
			return
		}

		user := serv.users[0]

		username := "qlite"

		pAndC := make([]byte, len(user.password)+len(splits[1]))

		copy(pAndC, user.password)
		copy(pAndC[len(user.password):], splits[1])

		pass, err := bcrypt.GenerateFromPassword(pAndC, bcrypt.DefaultCost)

		if err != nil {
			t.Errorf("failed to encrypt password")
			return
		}

		response := make([]byte, 2+len(username)+len(pass))

		response[0] = 0
		copy(response[1:], username)
		response[len(username)+1] = ';'
		copy(response[len(username)+2:], pass)

		alive = writeMessage(clientConn, response, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		mess, alive = readMessage(clientConn, serv.maxIoSeconds)

		mess = bytes.ToLower(mess)

		serverState := <-reqChan

		if serverState.isAlive {
			t.Error("connection is open unexpectedly")
			return
		}

		if !bytes.Contains(mess, []byte("error")) {
			t.Error("message is not an error")
		}

		if serverState.counter != nil {
			t.Error("counter was incremented")
			return
		}

	})

	t.Run("test invalid password", func(t *testing.T) {
		serv := Server{
			maxIoSeconds:             time.Second * 3,
			maxSubscriberConnections: 3,
			maxPublisherConnections:  3,
			users:                    users,
		}

		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
		}()

		reqChan := make(chan reqState)
		go func() {
			alive, role, counter := authenticate(&serv, serverConn)
			reqChan <- reqState{
				alive,
				role,
				counter,
			}
		}()

		mess, alive := readMessage(clientConn, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		splits := bytes.Split(mess, []byte(";"))

		if len(splits) != 2 {
			t.Errorf("improper message was sent %v", mess)
			return
		}

		challenge := "challenge"

		user := serv.users[0]

		pAndC := make([]byte, len(user.password)+len(challenge))

		copy(pAndC, user.password)
		copy(pAndC[len(user.password):], challenge)

		pass, err := bcrypt.GenerateFromPassword(pAndC, bcrypt.DefaultCost)

		if err != nil {
			t.Errorf("failed to encrypt password")
			return
		}

		response := make([]byte, 2+len(user.name)+len(pass))

		response[0] = 0
		copy(response[1:], user.name)
		response[len(user.name)+1] = ';'
		copy(response[len(user.name)+2:], pass)

		alive = writeMessage(clientConn, response, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		mess, alive = readMessage(clientConn, serv.maxIoSeconds)

		mess = bytes.ToLower(mess)

		serverState := <-reqChan

		if serverState.isAlive {
			t.Error("connection is open unexpectedly")
			return
		}

		if !bytes.Contains(mess, []byte("error")) {
			t.Error("message is not an error")
		}

		if serverState.counter != nil {
			t.Error("counter was incremented")
			return
		}

	})

	t.Run("test invalid role", func(t *testing.T) {
		serv := Server{
			maxIoSeconds:             time.Second * 3,
			maxSubscriberConnections: 3,
			maxPublisherConnections:  3,
			users:                    users,
		}

		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
		}()

		reqChan := make(chan reqState)
		go func() {
			alive, role, counter := authenticate(&serv, serverConn)
			reqChan <- reqState{
				alive,
				role,
				counter,
			}
		}()

		mess, alive := readMessage(clientConn, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		splits := bytes.Split(mess, []byte(";"))

		if len(splits) != 2 {
			t.Errorf("improper message was sent")
			return
		}

		user := serv.users[1]

		pAndC := make([]byte, len(user.password)+len(splits[1]))

		copy(pAndC, user.password)
		copy(pAndC[len(user.password):], splits[1])

		pass, err := bcrypt.GenerateFromPassword(pAndC, bcrypt.DefaultCost)

		if err != nil {
			t.Errorf("failed to encrypt password")
			return
		}

		response := make([]byte, 2+len(user.name)+len(pass))

		response[0] = 0
		copy(response[1:], user.name)
		response[len(user.name)+1] = ';'
		copy(response[len(user.name)+2:], pass)

		alive = writeMessage(clientConn, response, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		mess, alive = readMessage(clientConn, serv.maxIoSeconds)

		mess = bytes.ToLower(mess)

		serverState := <-reqChan

		if serverState.isAlive {
			t.Error("connection is open unexpectedly")
			return
		}

		if !bytes.Contains(mess, []byte("error")) {
			t.Error("message is not an error")
		}

		if serverState.counter != nil {
			t.Error("counter was incremented")
			return
		}
	})

	t.Run("test max roles filled", func(t *testing.T) {
		serv := Server{
			maxIoSeconds:             time.Second * 3,
			maxSubscriberConnections: 3,
			maxPublisherConnections:  3,
			users:                    users,
		}

		serv.currentPublisher.Store(3)

		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
		}()

		reqChan := make(chan reqState)
		go func() {
			alive, role, counter := authenticate(&serv, serverConn)
			reqChan <- reqState{
				alive,
				role,
				counter,
			}
		}()

		mess, alive := readMessage(clientConn, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		splits := bytes.Split(mess, []byte(";"))

		if len(splits) != 2 {
			t.Errorf("improper message was sent")
			return
		}

		user := serv.users[0]

		pAndC := make([]byte, len(user.password)+len(splits[1]))

		copy(pAndC, user.password)
		copy(pAndC[len(user.password):], splits[1])

		pass, err := bcrypt.GenerateFromPassword(pAndC, bcrypt.DefaultCost)

		if err != nil {
			t.Errorf("failed to encrypt password")
			return
		}

		response := make([]byte, 2+len(user.name)+len(pass))

		response[0] = 0
		copy(response[1:], user.name)
		response[len(user.name)+1] = ';'
		copy(response[len(user.name)+2:], pass)

		alive = writeMessage(clientConn, response, serv.maxIoSeconds)

		if !alive {
			t.Error("connection was closed unexpectedly")
			return
		}

		mess, alive = readMessage(clientConn, serv.maxIoSeconds)

		mess = bytes.ToLower(mess)

		serverState := <-reqChan

		if serverState.isAlive {
			t.Error("connection is open unexpectedly")
			return
		}

		if !bytes.Contains(mess, []byte("error")) {
			t.Error("message is not an error")
		}

		if serverState.counter != nil {
			t.Error("counter was incremented")
			return
		}
	})
}
