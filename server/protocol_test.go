package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"github.com/google/uuid"
	"testing"
	"time"
)

func Test_parseMessage(t *testing.T) {

	t.Run("test function only message", func(t *testing.T) {
		message := []byte("LEN;")

		err, function, data := parseMessage(message)

		if len(data) != 0 {
			t.Error("data should be empty")
		}

		if err != nil {
			t.Error(err)
		}

		if function != "LEN" {
			t.Error("parsed incorrectly")
		}
	})

	t.Run("test function only and parameter message", func(t *testing.T) {
		message := []byte("PUSH;1234")

		err, function, data := parseMessage(message)

		if len(data) != 4 {
			t.Error("parsed incorrectly")
		}

		if !bytes.Equal(data, []byte("1234")) {
			t.Error("parsed incorrectly")
		}

		if err != nil {
			t.Error(err)
		}

		if function != "PUSH" {
			t.Error("parsed incorrectly")
		}
	})

	t.Run("test too many semicolons", func(t *testing.T) {
		message := []byte("PUSH;123;4")

		err, function, data := parseMessage(message)

		if len(data) != 5 {
			t.Error("parsed incorrectly")
		}

		if !bytes.Equal(data, []byte("123;4")) {
			t.Error("parsed incorrectly")
		}

		if err != nil {
			t.Error(err)
		}

		if function != "PUSH" {
			t.Error("parsed incorrectly")
		}
	})

	t.Run("test no semicolons", func(t *testing.T) {
		message := []byte("PUSH1234")

		err, _, _ := parseMessage(message)

		if err == nil {
			t.Error("accepted invalid data")
		}
	})
}

func Test_handlePush(t *testing.T) {

	routines := 50

	duration := time.Second * 1

	type body struct {
		Typ         string `json:"typ"`
		Val         int    `json:"val"`
		Description string `json:"description"`
		Timestamp   int64  `json:"timestamp"`
	}

	reqBody := &body{
		Typ:         "test",
		Val:         100,
		Description: "test value",
		Timestamp:   time.Now().Unix(),
	}

	bodyBytes, err := json.Marshal(reqBody)

	if err != nil {
		t.Fatal(err)
	}

	t.Run("write valid message", func(t *testing.T) {
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

		q := NewMSQueue(1000, time.Second*100)
		alive := handlePush(serverConn, bodyBytes, q, duration)

		if !alive {
			t.Fatal("failed to write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Fatal("failed to read")
		}

		if q.Len() != 1 {
			t.Errorf("invalid qu size expected 1 got %d", q.Len())
		}

		splits := bytes.Split(message, []byte(";"))

		if len(splits) != 2 {
			t.Error("invalid response received")
		}

		if !bytes.Equal(splits[0], []byte("PASS")) {
			t.Error("invalid response received")
		}

		pos := binary.LittleEndian.Uint32(splits[1])

		if pos != 1 {
			t.Error("invalid position received")
		}
	})

	t.Run("write multiple valid message", func(t *testing.T) {
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

		q := NewMSQueue(1000, time.Second*100)

		for range routines {
			alive := handlePush(serverConn, bodyBytes, q, duration)

			if !alive {
				t.Error("failed write")
			}

			message, alive := readMessage(clientConn, duration)

			if !alive {
				t.Error("failed read")
			}

			splits := bytes.Split(message, []byte(";"))

			if len(splits) != 2 {
				t.Error("invalid response received")
			}

			if !bytes.Equal(splits[0], []byte("PASS")) {
				t.Error("invalid response received")
			}
		}

		if q.Len() != int32(routines) {
			t.Error("not all messages were pushed")
		}
	})

	t.Run("write invalid message", func(t *testing.T) {
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

		q := NewMSQueue(1000, time.Second*100)
		alive := handlePush(serverConn, []byte{}, q, duration)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		splits := bytes.Split(message, []byte(";"))

		if len(splits) != 2 {
			t.Error("invalid response received")
		}

		if !bytes.Equal(splits[0], []byte("FAIL")) {
			t.Error("invalid response received")
		}
	})
}

func Test_handleHide(t *testing.T) {
	const routines = 50
	duration := time.Second * 1
	maxHiddenTime := time.Duration(10) * time.Second

	t.Run("valid pop", func(t *testing.T) {
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

		q := NewMSQueue(1000, time.Second*100)
		defer q.Kill()

		_, err = q.Push([]byte("123"))
		if err != nil {
			t.Fatal(err)
		}

		alive := handleHide(serverConn, q, maxHiddenTime, duration)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		before, after, found := bytes.Cut(message, []byte(";"))

		if !found {
			t.Fatal("could not extract response")
		}

		if !bytes.Equal(before, []byte("PASS")) {
			t.Error("invalid response received")
		}

		uid, err := uuid.FromBytes(after[:16])

		if err != nil {
			t.Fatal(err)
		}

		mess := after[17:]

		if q.Len() != 0 {
			t.Error("qu was not updated")
		}

		if !bytes.Equal(mess, []byte("123")) {
			t.Error("invalid response received")
		}

		if err = q.Delete(uid); err != nil {
			t.Error(err)
		}
	})

	t.Run("multiple pops", func(t *testing.T) {
		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		q := NewMSQueue(1000, time.Second*100)

		defer func() {
			q.Kill()
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
		}()

		for range routines {
			_, err = q.Push([]byte("123"))
			if err != nil {
				t.Fatal(err)
			}
		}

		for idx := range routines {
			alive := handleHide(serverConn, q, maxHiddenTime, duration)
			if !alive {
				t.Error("failed write")
			}

			_, alive = readMessage(clientConn, duration)

			if !alive {
				t.Error("failed read")
			}

			if q.Len() != int32(routines-1-idx) {
				t.Errorf("qu was not properly pushed")
			}
		}
	})

	t.Run("pop from empty queue", func(t *testing.T) {
		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		q := NewMSQueue(1000, time.Second*100)

		defer func() {
			q.Kill()
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
		}()

		alive := handleHide(serverConn, q, maxHiddenTime, duration)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		splits := bytes.Split(message, []byte(";"))

		if len(splits) != 2 {
			t.Error("invalid response received")
		}

		if !bytes.Equal(splits[0], []byte("FAIL")) {
			t.Error("invalid response received")
		}
	})

	t.Run("test request cancellation", func(t *testing.T) {
		ts := newTestServer()
		serverConn, _, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		_ = serverConn.Close()

		q := NewMSQueue(1000, time.Second*100)

		defer func() {
			q.Kill()
			_ = serverConn.Close()
			ts.endServer(t)
		}()

		_, err = q.Push([]byte("123"))
		if err != nil {
			t.Fatal(err)
		}

		alive := handleHide(serverConn, q, maxHiddenTime, duration)

		if alive {
			t.Error("write succeeded")
		}

		tout, cf := context.WithTimeout(context.Background(), time.Second*1)
		defer cf()

		ch := make(chan struct{})
		alive = true

		go func() {
			for q.Len() == 0 && alive {
			}

			if !alive {
				return
			}

			if q.Len() == 1 {
				ch <- struct{}{}
			}
		}()

		select {
		case <-ch:
			return
		case <-tout.Done():
			alive = false
			t.Fatal(context.DeadlineExceeded)
		}
	})
}

func Test_handlePoll(t *testing.T) {
	duration := time.Second * 1
	pollingTime := time.Second * 1
	hiddenDuration := time.Second * 20

	t.Run("test poll", func(t *testing.T) {
		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		q := NewMSQueue(1000, time.Second*100)

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
			q.Kill()
		}()
		_, err = q.Push([]byte("1234"))

		if err != nil {
			t.Error(err)
		}

		alive := handlePoll(serverConn, q, duration, hiddenDuration, pollingTime)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		before, after, found := bytes.Cut(message, []byte(";"))

		if !found {
			t.Fatal("could not accurately split message")
		}

		if !bytes.Equal(before, []byte("PASS")) {
			t.Error("invalid response received")
		}

		if q.Len() != 0 {
			t.Error("queue was not updated")
		}

		if !bytes.Equal(after[17:], []byte("1234")) {
			t.Error("invalid response received")
		}
	})

	t.Run("test longPop post filled", func(t *testing.T) {
		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		q := NewMSQueue(1000, time.Second*100)

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
			q.Kill()
		}()

		go func() {
			time.Sleep(500 * time.Millisecond)
			_, err = q.Push([]byte("1234"))

			if err != nil {
				t.Error(err)
			}
		}()

		alive := handlePoll(serverConn, q, duration, hiddenDuration, pollingTime)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		before, after, found := bytes.Cut(message, []byte(";"))

		if !found {
			t.Fatal("could not accurately split message")
		}

		if !bytes.Equal(before, []byte("PASS")) {
			t.Error("invalid response received")
		}

		if q.Len() != 0 {
			t.Error("queue was not updated")
		}

		if !bytes.Equal(after[17:], []byte("1234")) {
			t.Error("invalid response received")
		}
	})

	t.Run("test longPop failed", func(t *testing.T) {
		ts := newTestServer()
		serverConn, clientConn, err := ts.getServerConn()

		if err != nil {
			t.Fatal(err)
		}

		q := NewMSQueue(1000, time.Second*100)

		defer func() {
			_ = serverConn.Close()
			_ = clientConn.Close()
			ts.endServer(t)
			q.Kill()
		}()

		alive := handlePoll(serverConn, q, duration, hiddenDuration, pollingTime)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		splits := bytes.Split(message, []byte(";"))

		if !bytes.Equal(splits[0], []byte("FAIL")) {
			t.Error("invalid response received")
		}
	})
}

func Test_len(t *testing.T) {
	duration := time.Second * 1

	ts := newTestServer()
	serverConn, clientConn, err := ts.getServerConn()

	if err != nil {
		t.Fatal(err)
	}

	q := NewMSQueue(1000, time.Second*100)

	defer func() {
		_ = serverConn.Close()
		_ = clientConn.Close()
		ts.endServer(t)
		q.Kill()
	}()

	_, err = q.Push([]byte("12354"))
	if err != nil {
		t.Error(err)
	}

	_, err = q.Push([]byte("12354"))
	if err != nil {
		t.Error(err)
	}

	alive := handleLen(serverConn, q, time.Millisecond*100)

	if !alive {
		t.Error("failed write")
	}

	message, alive := readMessage(clientConn, duration)

	if !alive {
		t.Error("failed read")
	}

	splits := bytes.Split(message, []byte(";"))

	if len(splits) != 2 {
		t.Fatal("invalid response received")
	}

	if !bytes.Equal(splits[0], []byte("PASS")) {
		t.Error("invalid response received")
	}

	if uint32(q.Len()) != binary.LittleEndian.Uint32(splits[1]) {
		t.Error("invalid len received")
	}
}
