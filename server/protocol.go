package server

import (
	"benchai/qlite/queue"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

func receiveRequests(conn net.Conn, server Server, q *queue.Queue, role string) {
	for {
		message, alive := readMessage(conn, server.maxIoSeconds)

		if !alive {
			return
		}

		err, function, data := parseMessage(message)

		function = strings.ToUpper(function)

		if err != nil {
			if !writeError(conn, err, server.maxIoSeconds) {
				return
			}
			continue
		}

		switch function {
		case "PUSH":
			if role != "publisher" {
				err = errors.New("subscribers cannot push to the queue")
				alive = writeError(conn, err, server.maxIoSeconds)
			} else {
				alive = handlePush(conn, data, q, server.maxIoSeconds)
			}
		case "SPOP":
			if role != "subscriber" {
				err = errors.New("publishers cannot pop from the queue")
				alive = writeError(conn, err, server.maxIoSeconds)
			} else {
				alive = handleShortPop(conn, q, server.maxIoSeconds)
			}
		case "LPOP":
			if role != "subscriber" {
				err = errors.New("publishers cannot pop from the queue")
				alive = writeError(conn, err, server.maxIoSeconds)
			} else {
				alive = handleLongPop(conn, q, server.maxIoSeconds, server.pollingTimeSeconds)
			}
		case "LEN":
			alive = handleLen(conn, q, server.maxIoSeconds)
		default:
			err = fmt.Errorf(
				"error: error can not handle function %s, options are PUSH, LPOP, SPOP and LEN",
				function)

			alive = writeError(conn, err, server.maxIoSeconds)
		}

		if !alive {
			return
		}
	}
}

/*
parseMessage

extracts the function being requested, and it's subsequent arguments.
*/
func parseMessage(message []byte) (err error, function string, data []byte) {

	before, data, found := bytes.Cut(message, []byte(";"))

	if !found {
		err = errors.New("no semicolon delimiter found")
		return
	}

	if len(before) == 0 {
		err = errors.New("error: bad request")
		return
	}

	function = string(before)

	return
}

/*
handlePush

adds the message to the queue
*/
func handlePush(conn net.Conn, messageData []byte, q *queue.Queue, deadline time.Duration) bool {
	if len(messageData) == 0 {
		err := errors.New("cannot push empty message")

		if !writeError(conn, err, deadline) {
			return false
		}

		return true
	}

	pos, err := q.Push(messageData)

	if err != nil {
		if !writeError(conn, err, deadline) {
			return false
		}

		return true
	}

	posSlice := make([]byte, 4)
	binary.LittleEndian.PutUint32(posSlice, pos)
	return writeSuccess(conn, posSlice, deadline)
}

/*
handleShortPop

pops the first element off the queue
*/
func handleShortPop(conn net.Conn, q *queue.Queue, deadline time.Duration) bool {

	message, err := q.Pop()

	if err != nil {
		if !writeError(conn, err, deadline) {
			return false
		}

		return true
	}

	return writeSuccess(conn, message, deadline)
}

func handleLen(conn net.Conn, q *queue.Queue, deadline time.Duration) bool {
	length := q.Len()
	posSlice := make([]byte, 4)
	binary.LittleEndian.PutUint32(posSlice, uint32(length))
	return writeSuccess(conn, posSlice, deadline)
}

func handleLongPop(conn net.Conn, q *queue.Queue, deadline time.Duration, pollingTime time.Duration) bool {

	type packet struct {
		err     error
		message []byte
	}

	messageChan := make(chan packet)
	stopChan := make(chan struct{}) // stops the reading of the queue

	go func() {
		for { // read from the queue until the queue pops or a stop signal is received
			message, err := q.Pop()
			mess := packet{err: err, message: message}

			select {
			case <-stopChan:
				messageChan <- mess
			default:
				if mess.err == nil {
					messageChan <- mess
				}
			}
		}
	}()

	ctx, cFunc := context.WithDeadline(context.Background(), time.Now().Add(pollingTime))
	defer cFunc()

	var mess packet

	select {
	case <-ctx.Done():
		stopChan <- struct{}{} // stop the reading
		mess = <-messageChan   // retrieve the last message
		if mess.err != nil {
			mess.err = errors.New("error: no data could be dequeued in the specified time period")
		}
	case mess = <-messageChan: // queue popped before deadline
	}

	if mess.err != nil {
		if !writeError(conn, mess.err, deadline) {
			return false
		}
		return true
	}

	return writeSuccess(conn, mess.message, deadline)
}
