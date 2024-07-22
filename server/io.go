package server

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log"
	"net"
	"time"
)

const KB uint32 = 1024
const MB = KB * KB

/*
closeConn

close the connection and log the error message
*/
func closeConn(conn net.Conn) {
	err := conn.Close()
	if err != nil {
		log.Println(err)
	}
}

func gracefulShutdown(finished chan<- struct{}, connections *uniqueConnections, listener net.Listener) {

	for _, con := range connections.connMap {
		closeConn(con)
	}

	err := listener.Close()

	if err != nil {
		log.Println(err)
	}

	finished <- struct{}{}
}

/*
writeMessage

sends a message ove the connection prepended with the size of the message
*/
func writeMessage(conn net.Conn, message []byte, deadline time.Duration) (alive bool) {

	if len(message) > int(10*MB) {
		panic("writing message larger than 10MB")
	}

	size := make([]byte, 4)
	binary.LittleEndian.PutUint32(size, uint32(len(message))) // encode the size of the message

	fullMessage := make([]byte, 0, 4+len(message))
	fullMessage = append(fullMessage, size...)    // add message size as the first 4 bytes
	fullMessage = append(fullMessage, message...) // add the message after

	written := 0
	for written < len(fullMessage) {
		err := conn.SetWriteDeadline(time.Now().Add(deadline)) // set write deadline

		if err != nil {
			closeConn(conn)
			return false
		}

		n, err := conn.Write(fullMessage[written:])

		if err != nil {
			closeConn(conn)
			return false
		}

		written += n
	}

	err := conn.SetWriteDeadline(time.Time{}) // remove deadline for future connections

	if err != nil {
		closeConn(conn)
		return false
	}

	return true
}

/*
readMessageLength

extracts the length of client message, this length is then used to determine when the message is done being
read
*/
func readMessageLength(conn net.Conn, deadline time.Duration) (length uint32, alive bool) {
	size := make([]byte, 0, 4)
	sizeBuffer := make([]byte, 4) // buffer that will hold the size of the message
	n, err := conn.Read(sizeBuffer)

	if err != nil {
		logError(conn, err)
		return
	}

	pos := n
	size = append(size, sizeBuffer[:pos]...)

	err = conn.SetReadDeadline(time.Now().Add(deadline)) // entire message must be read in this time

	if err != nil {
		logError(conn, err)
		return
	}

	for pos < 4 { // read the remaining size bytes if needed
		sizeBuffer = make([]byte, 4-pos)
		n, err = conn.Read(sizeBuffer)

		if err != nil {
			logError(conn, err)
			return
		}

		size = append(size, sizeBuffer[:n]...)
		pos += n
	}

	length = binary.LittleEndian.Uint32(size)
	if length > 10*MB {
		err = errors.New("error: message is longer than limit of 10 megabytes")
		writeCriticalError(conn, err, deadline)
		return 0, false
	}

	err = conn.SetReadDeadline(time.Time{}) // remove deadline for future connections
	if err != nil {
		logError(conn, err)
		return
	}

	alive = true
	return
}

/*
readMessage

reads a client side message
*/
func readMessage(conn net.Conn, deadline time.Duration) (message []byte, alive bool) {

	length, status := readMessageLength(conn, deadline)

	if !status {
		return nil, false
	}

	fullMessage := make([]byte, 0, int(length)) // preallocate required size for message
	buffer := make([]byte, 1024)
	err := conn.SetReadDeadline(time.Now().Add(deadline)) // entire message must be read in this time

	if err != nil {
		logError(conn, err)
		return
	}

	pos := 0
	for pos < int(length) { // collect the entire message
		n, err := conn.Read(buffer)

		if err != nil {
			logError(conn, err)
			return
		}

		pos += n

		if pos > int(length) {
			err = errors.New("error: received message length longer then specified")
			writeCriticalError(conn, err, deadline)
			return
		}

		fullMessage = append(fullMessage, buffer[:n]...)
	}

	err = conn.SetReadDeadline(time.Time{}) // remove deadline for future connections

	if err != nil {
		logError(conn, err)
		return
	}

	return fullMessage, true
}

func connectionFull(
	user User,
	conn net.Conn,
	deadline time.Duration) {

	name := func() string {
		if user.publisher {
			return "publisher"
		}
		return "subscriber"
	}

	err := fmt.Errorf("error: could not establish connection since %s limit has been reached", name())
	writeCriticalError(conn, err, deadline)
}

/*
writeCriticalError

writes an error and closes the connection
*/
func writeCriticalError(conn net.Conn, err error, deadline time.Duration) {
	if writeError(conn, err, deadline) {
		closeConn(conn)
	}
}

func writeError(conn net.Conn, err error, deadline time.Duration) bool {
	err = fmt.Errorf("FAIL;%v", err)
	return writeMessage(conn, []byte(err.Error()), deadline)
}

func writeSuccess(conn net.Conn, message []byte, deadline time.Duration) bool {
	start := []byte("PASS;")
	full := make([]byte, len(start)+len(message))

	copy(full, start)
	copy(full[len(start):], message)

	if !writeMessage(conn, full, deadline) {
		return false
	}

	return true
}

func logError(conn net.Conn, err error) {
	log.Printf("experienced error: %v", err)
	closeConn(conn)
}
