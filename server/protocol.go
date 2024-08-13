package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"net"
	"strings"
	"time"
)

func receiveRequests(conn net.Conn, server *Server, q *Queue, role string) {
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
				err = errors.New("subscribers cannot push to the qu")
				alive = writeError(conn, err, server.maxIoSeconds)
			} else {
				alive = handlePush(conn, data, q, server.maxIoSeconds)
			}
		case "HIDE":
			if role != "subscriber" {
				err = errors.New("publishers cannot pop from the qu")
				alive = writeError(conn, err, server.maxIoSeconds)
			} else {
				hiddenDuration := convertBytesToSeconds(data)
				alive = handleHide(conn, q, hiddenDuration, server.maxIoSeconds)
			}
		case "POLL":
			if role != "subscriber" {
				err = errors.New("publishers cannot pop from the qu")
				alive = writeError(conn, err, server.maxIoSeconds)
			} else {
				hiddenDuration := convertBytesToSeconds(data)
				alive = handlePoll(conn, q, server.maxIoSeconds, hiddenDuration, server.pollingTimeSeconds)
			}
		case "LEN":
			alive = handleLen(conn, q, server.maxIoSeconds)
		case "DEL":
			uid, err := uuid.FromBytes(data)
			if err != nil {
				alive = writeError(conn, err, server.maxIoSeconds)
			} else {
				alive = handleDel(conn, q, server.maxIoSeconds, uid)
			}
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

func convertBytesToSeconds(mess []byte) time.Duration {
	seconds := binary.LittleEndian.Uint32(mess)
	return time.Duration(seconds) * time.Second
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
func handlePush(conn net.Conn, messageData []byte, q *Queue, deadline time.Duration) bool {
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
	binary.LittleEndian.PutUint32(posSlice, uint32(pos))
	return writeSuccess(conn, posSlice, deadline)
}

/*
createPopResponse

formats the pop response message
*/
func createPopResponse(uid uuid.UUID, mess []byte) []byte {
	wholeMessage := make([]byte, len(mess)+17)
	copy(wholeMessage, uid[:])
	wholeMessage[len(uid)] = ';'
	copy(wholeMessage[len(uid)+1:], mess)

	return wholeMessage
}

/*
writePop

tells the client the message has been hidden while sending it over, if the connection is closed the hidden message is
reinstated
*/
func writePop(conn net.Conn, deadline time.Duration, mess []byte, uid uuid.UUID, q *Queue) bool {
	state := writeSuccess(conn, mess, deadline)
	if !state {
		_ = q.cancel(uid)
	}

	return state
}

/*
handleHide

hides the first message in the queue if available for a certain duration
*/
func handleHide(conn net.Conn, q *Queue, hiddenDuration, deadline time.Duration) bool {
	message, uid, err := q.Hide(hiddenDuration)
	if err != nil {
		if !writeError(conn, err, deadline) {
			return false
		}
		return true
	}

	return writePop(conn, deadline, createPopResponse(uid, message), uid, q)
}

func handleLen(conn net.Conn, q *Queue, deadline time.Duration) bool {
	length := q.Len()
	posSlice := make([]byte, 4)
	binary.LittleEndian.PutUint32(posSlice, uint32(length))
	return writeSuccess(conn, posSlice, deadline)
}

func handlePoll(conn net.Conn, q *Queue, deadline, hiddenDuration, pollingTime time.Duration) bool {

	type packet struct {
		err     error
		message []byte
		uid     uuid.UUID
	}

	messageChan := make(chan packet)
	stopChan := make(chan struct{}) // stops the reading of the queue

	go func() {
		for { // read from the qu until the qu pops or a stop signal is received
			message, uid, err := q.Hide(hiddenDuration)
			mess := packet{err: err, message: message, uid: uid}

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

	ctx, cFunc := context.WithTimeout(context.Background(), pollingTime)
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

	return writePop(conn, deadline, createPopResponse(mess.uid, mess.message), mess.uid, q)
}

func handleDel(conn net.Conn, q *Queue, deadline time.Duration, uid uuid.UUID) bool {
	err := q.Delete(uid)

	if err != nil {
		return writeError(conn, err, deadline)
	}

	return writeSuccess(conn, uid[:], deadline)
}
