package server

import (
	"benchai/qlite/queue"
	"bytes"
	"encoding/binary"
	"encoding/json"
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

	var maxMessSize uint32 = 200
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

		q := queue.NewQueue(1000, maxMessSize)
		alive := handlePush(serverConn, bodyBytes, &q, duration)

		if !alive {
			t.Fatal("failed to write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Fatal("failed to read")
		}

		if q.Len() != 1 {
			t.Errorf("invalid queue size expected 1 got %d", q.Len())
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

		q := queue.NewQueue(1000, maxMessSize)

		for range routines {
			alive := handlePush(serverConn, bodyBytes, &q, duration)

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

		q := queue.NewQueue(1000, maxMessSize)
		alive := handlePush(serverConn, []byte{}, &q, duration)

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

	t.Run("write to full queue", func(t *testing.T) {
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

		q := queue.NewQueue(1, maxMessSize)

		_, err = q.Push([]byte("123"))

		if err != nil {
			t.Fatal(err)
		}

		alive := handlePush(serverConn, []byte{}, &q, duration)

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

func Test_shortPop(t *testing.T) {
	var maxMessSize uint32 = 200
	const routines = 50
	duration := time.Second * 1

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

		q := queue.NewQueue(1, maxMessSize)
		_, err = q.Push([]byte("123"))
		if err != nil {
			t.Fatal(err)
		}

		alive := handleShortPop(serverConn, &q, duration)

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

		if q.Len() != 0 {
			t.Error("queue was not updated")
		}

		if !bytes.Equal(splits[1], []byte("123")) {
			t.Error("invalid response received")
		}
	})

	t.Run("multiple pops", func(t *testing.T) {
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

		q := queue.NewQueue(50, maxMessSize)

		for range routines {
			_, err = q.Push([]byte("123"))
			if err != nil {
				t.Fatal(err)
			}
		}

		for idx := range routines {
			alive := handleShortPop(serverConn, &q, duration)
			if !alive {
				t.Error("failed write")
			}

			_, alive = readMessage(clientConn, duration)

			if !alive {
				t.Error("failed read")
			}

			if q.Len() != int32(routines-1-idx) {
				t.Errorf("queue was not properly pushed")
			}

		}
	})

	t.Run("pop from empty queue", func(t *testing.T) {
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

		q := queue.NewQueue(1, maxMessSize)

		alive := handleShortPop(serverConn, &q, duration)

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

func Test_longPop(t *testing.T) {
	var maxMessSize uint32 = 200
	duration := time.Second * 1

	t.Run("test longPop prefilled", func(t *testing.T) {
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

		q := queue.NewQueue(1, maxMessSize)
		_, err = q.Push([]byte("1234"))

		if err != nil {
			t.Error(err)
		}

		alive := handleLongPop(serverConn, &q, duration, time.Second*5)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		splits := bytes.Split(message, []byte(";"))

		if !bytes.Equal(splits[0], []byte("PASS")) {
			t.Error("invalid response received")
		}

		if q.Len() != 0 {
			t.Error("queue was not updated")
		}

		if !bytes.Equal(splits[1], []byte("1234")) {
			t.Error("invalid response received")
		}
	})

	t.Run("test longPop post filled", func(t *testing.T) {
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

		q := queue.NewQueue(1, maxMessSize)

		if err != nil {
			t.Error(err)
		}

		go func() {
			time.Sleep(500 * time.Millisecond)
			_, err = q.Push([]byte("1234"))

			if err != nil {
				t.Error(err)
			}
		}()

		alive := handleLongPop(serverConn, &q, duration, time.Second*2)

		if !alive {
			t.Error("failed write")
		}

		message, alive := readMessage(clientConn, duration)

		if !alive {
			t.Error("failed read")
		}

		splits := bytes.Split(message, []byte(";"))

		if !bytes.Equal(splits[0], []byte("PASS")) {
			t.Error("invalid response received")
		}

		if q.Len() != 0 {
			t.Error("queue was not updated")
		}

		if !bytes.Equal(splits[1], []byte("1234")) {
			t.Error("invalid response received")
		}
	})

	t.Run("test longPop failed", func(t *testing.T) {
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

		q := queue.NewQueue(1, maxMessSize)

		alive := handleLongPop(serverConn, &q, time.Second*10, time.Millisecond*100)

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
	var maxMessSize uint32 = 200
	duration := time.Second * 1

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

	q := queue.NewQueue(10, maxMessSize)
	_, err = q.Push([]byte("12354"))
	if err != nil {
		t.Error(err)
	}

	_, err = q.Push([]byte("12354"))
	if err != nil {
		t.Error(err)
	}

	alive := handleLen(serverConn, &q, time.Millisecond*100)

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
