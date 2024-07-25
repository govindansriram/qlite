package server

import (
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
