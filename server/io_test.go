package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"
)

type testServer struct {
	kill     chan struct{}
	connChan chan net.Conn
	errChan  chan error
}

func newTestServer() testServer {
	killChannel := make(chan struct{})
	connectionChannel := make(chan net.Conn)
	errorChannel := make(chan error)

	return testServer{
		kill:     killChannel,
		connChan: connectionChannel,
		errChan:  errorChannel,
	}
}

func (t testServer) startServer() {

	listener, err := net.Listen("tcp", "localhost:8080")

	if err != nil {
		t.errChan <- err
		return
	}

	connectionChannel := make(chan net.Conn)
	errorChannel := make(chan error)

	go func() {
		serverConn, err := listener.Accept()

		if err != nil {
			_ = listener.Close()
			errorChannel <- err
			return
		}

		connectionChannel <- serverConn
	}()

	var serverConn net.Conn
	var brk bool

	for {
		select {
		case err = <-errorChannel:
			t.errChan <- err
			return
		case serverConn = <-connectionChannel:
			t.connChan <- serverConn
			brk = true
		}

		if brk {
			break
		}
	}

	defer func() {
		_ = serverConn.Close()
		_ = listener.Close()
	}()

	for {
		select {
		case <-t.kill:
			_ = serverConn.Close()
			err = listener.Close()
			t.errChan <- err
			return
		default:
		}
	}
}

func (t testServer) getServerConn() (server net.Conn, client net.Conn, err error) {
	go t.startServer()

	clientChan := make(chan net.Conn)
	clientErrChan := make(chan error)
	for {
		go func() {
			clientConn, err := net.Dial("tcp", "localhost:8080")
			if err == nil {
				clientChan <- clientConn
			} else {
				clientErrChan <- err
			}
		}()

		select {
		case client = <-clientChan:
			server = <-t.connChan
			return server, client, err
		case err = <-t.errChan:
			return nil, nil, err
		case <-clientErrChan:
			select {
			case <-t.errChan:
				return nil, nil, err
			default:
			}
		}
	}
}

func (t testServer) endServer(tt *testing.T) {

	t.kill <- struct{}{}

	if err := <-t.errChan; err != nil {
		tt.Error(err)
	}

	return
}

func Test_closeConn(t *testing.T) {
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

	closeConn(serverConn)
	_, err = serverConn.Write([]byte{1, 3, 7})

	if err == nil {
		t.Error("connection was not closed")
	}

	_, _ = clientConn.Write([]byte("somed ata"))
	_, err = clientConn.Write([]byte("somed ata"))

	if err == nil {
		t.Error("connection was not closed")
	}

	_ = clientConn.Close()
}

func readAll(messageSize int, client net.Conn) ([]byte, error) {
	wholeMessage := make([]byte, 0, messageSize+4)

	buffer := make([]byte, 100)
	for len(wholeMessage) < messageSize {
		n, err := client.Read(buffer)

		if err != nil {
			return nil, err
		}

		wholeMessage = append(wholeMessage, buffer[:n]...)
	}

	return wholeMessage, nil
}

func writeAll(message []byte, client net.Conn, waitTime *time.Duration) error {
	request := make([]byte, 4)
	binary.LittleEndian.PutUint32(request, uint32(len(message)))
	request = append(request, message...)

	pos := 0
	for pos < len(request) {
		newSlice := make([]byte, 1)
		copy(newSlice, request[pos:pos+1])
		n, err := client.Write(newSlice)

		if err != nil {
			return err
		}

		if waitTime != nil {
			time.Sleep(*waitTime)
		}

		pos += n
	}

	return nil
}

func collectMessage(limit time.Duration, messageSize int, client net.Conn) ([]byte, error) {
	type packet struct {
		mess []byte
		err  error
	}

	packets := make(chan packet)

	go func() {
		mess, err := readAll(messageSize, client)
		pack := packet{mess: mess, err: err}
		packets <- pack
	}()

	deadline := time.Now().Add(limit)
	ctx, cancelFunc := context.WithDeadline(context.Background(), deadline)
	defer cancelFunc()

	select {
	case pack := <-packets:
		if pack.err != nil {
			return nil, pack.err
		}

		return pack.mess, nil

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func writeResponse(
	limit time.Duration,
	message []byte,
	client net.Conn,
	waitTime *time.Duration) error {
	type packet struct {
		err error
	}

	packets := make(chan packet)

	go func() {
		err := writeAll(message, client, waitTime)
		pack := packet{err: err}
		packets <- pack
	}()

	deadline := time.Now().Add(limit)
	ctx, cancelFunc := context.WithDeadline(context.Background(), deadline)
	defer cancelFunc()

	select {
	case pack := <-packets:
		return pack.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func Test_writeConn(t *testing.T) {

	t.Run("test write standard", func(t *testing.T) {
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

		duration := time.Second * 3

		message := []byte("hello world")
		alive := writeMessage(serverConn, message, duration)

		if !alive {
			t.Error("connection was closed")
		}

		response, err := collectMessage(duration, len(message), clientConn)

		if err != nil {
			t.Error(err)
			return
		}

		messageLen := binary.LittleEndian.Uint32(response[:4])

		if int(messageLen) != len(message) {
			t.Error("lengths do not match")
		}
	})

	t.Run("test double write standard", func(t *testing.T) {
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

		duration := time.Millisecond * 100
		message := []byte("hello world")
		alive := writeMessage(serverConn, message, duration)

		if !alive {
			t.Fatal("connection was closed")
		}

		duration = time.Millisecond * 100

		time.Sleep(duration)

		alive = writeMessage(serverConn, message, duration)

		if !alive {
			t.Error("connection was closed after first write")
		}
	})
}

func Test_readMessageLength(t *testing.T) {

	t.Run("test readMessageLength with minor delays", func(t *testing.T) {
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

		duration := time.Second * 5
		message := []byte("hello world")
		aliveChan := make(chan bool)

		go func() {
			_, alive := readMessageLength(serverConn, time.Millisecond*500)
			aliveChan <- alive
		}()

		snoozeTime := time.Millisecond * 100
		err = writeResponse(duration, message, clientConn, &snoozeTime)

		if err != nil {
			t.Error("connection was closed")
		}

		if !<-aliveChan {
			t.Error("connection should not have expired")
			return
		}
	})

	t.Run("test readMessageLength over time limit", func(t *testing.T) {
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

		duration := time.Second * 5
		message := []byte("hello world")
		aliveChan := make(chan bool)

		go func() {
			_, alive := readMessageLength(serverConn, time.Millisecond*1)
			aliveChan <- alive
		}()

		snoozeTime := time.Millisecond * 100
		err = writeResponse(duration, message, clientConn, &snoozeTime)

		if err == nil {
			t.Error("connection was not closed")
		}

		if <-aliveChan {
			t.Error("Connection should have expired")
			return
		}
	})

	t.Run("test readMessageLength with > 10mb message", func(t *testing.T) {
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

		duration := time.Second * 3
		message := make([]byte, 10*MB+1)
		for index, _ := range message {
			message[index] = 0
		}

		aliveChan := make(chan bool)
		go func() {
			_, alive := readMessageLength(serverConn, time.Second*10)
			aliveChan <- alive
		}()

		_ = writeResponse(duration, message, clientConn, nil)
		if <-aliveChan {
			t.Error("Connection should have expired")
			return
		}
	})

	t.Run("test connection after successful readMessageLength", func(t *testing.T) {
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

		duration := time.Second * 1
		message := []byte("hello world")
		err = writeResponse(duration, message, clientConn, nil)

		if err != nil {
			t.Error("connection was closed")
		}

		_, alive := readMessageLength(serverConn, duration)
		if !alive {
			t.Error("Connection ended unexpectedly")
			return
		}

		time.Sleep(1 * time.Second)
		_, err = clientConn.Write([]byte{1, 1, 1})
		if err != nil {
			t.Error("connection was closed")
		}
	})

	t.Run("test connection after failed readMessageLength", func(t *testing.T) {
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

		duration := time.Second * 2

		message := []byte("hello world")
		wt := 100 * time.Millisecond

		go func() {
			time.Sleep(200 * time.Millisecond)
			closeConn(clientConn)
		}()

		err = writeResponse(duration, message, clientConn, &wt)
		if err == nil {
			t.Error("connection was not closed")
		}

		_, alive := readMessageLength(serverConn, duration)
		if alive {
			t.Error("connection is alive")
			return
		}

		_, err = serverConn.Write(make([]byte, 512))
		if err == nil {
			t.Error("connection is alive")
		}
	})
}

func Test_readMessage(t *testing.T) {
	type packet struct {
		message []byte
		alive   bool
	}

	t.Run("test standard read Message", func(t *testing.T) {
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

		duration := time.Second * 3

		packetChan := make(chan packet)

		go func() {
			message, alive := readMessage(serverConn, duration)
			packetChan <- packet{message: message, alive: alive}
		}()

		request := make([]byte, 2048)
		for idx := range 2048 {
			request[idx] = 1
		}

		pos := 0
		size := make([]byte, 4)
		binary.LittleEndian.PutUint32(size, 2048)
		mess := append(size, request...)

		for pos != len(mess) {
			n, err := clientConn.Write(mess[pos:])

			if err != nil {
				t.Error(err)
				return
			}

			pos += n
		}

		pack := <-packetChan

		if !pack.alive {
			t.Error("connection closed unexpectedly")
		}

		if !bytes.Equal(request, pack.message) {
			t.Error("message are not equal")
		}
	})

	t.Run("test early exit", func(t *testing.T) {
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

		duration := time.Second * 1

		packetChan := make(chan packet)

		go func() {
			message, alive := readMessage(serverConn, duration)
			packetChan <- packet{message: message, alive: alive}
		}()

		request := make([]byte, 2048)
		for idx := range 2048 {
			request[idx] = 1
		}

		pos := 0
		size := make([]byte, 4)
		binary.LittleEndian.PutUint32(size, 4096)
		mess := append(size, request...)

		for pos != len(mess) {
			n, err := clientConn.Write(mess[pos:])

			if err != nil {
				t.Error(err)
				return
			}

			pos += n
		}

		pack := <-packetChan

		if pack.alive {
			t.Error("connection remainde intact")
		}

		if bytes.Equal(request, pack.message) {
			t.Error("message are equal")
		}
	})

	t.Run("test exceed limit", func(t *testing.T) {
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

		duration := time.Second * 1

		packetChan := make(chan packet)

		go func() {
			message, alive := readMessage(serverConn, duration)
			packetChan <- packet{message: message, alive: alive}
		}()

		request := make([]byte, 2048)
		for idx := range 2048 {
			request[idx] = 1
		}

		pos := 0
		size := make([]byte, 4)
		binary.LittleEndian.PutUint32(size, 2041)
		mess := append(size, request...)

		for pos != len(mess) {
			n, err := clientConn.Write(mess[pos:])

			if err != nil {
				t.Error(err)
				return
			}

			pos += n
		}

		pack := <-packetChan

		if pack.alive {
			t.Error("connection remainde intact")
		}

		if bytes.Equal(request, pack.message) {
			t.Error("message are equal")
		}
	})

}
