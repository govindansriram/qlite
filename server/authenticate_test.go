package server

import (
	"bytes"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"testing"
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
