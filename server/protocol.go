package server

/*
# qlite Request & Response

## Request

  The first 4 bytes should be the ascii encoded name of the function you wish to execute.
  Your options are:

  - PUSH, writes a request to the end of the queue
  - POP, removes the first element in the queue, and returns it
  - LEN, returns the current length of the queue

  The next 4 bytes after a push request represent the size of the message being pushed.
  All other after that are within the size limit will then be considered as part of the message,
  these bytes must be UTF-8 encoded in json format. POP and LEN do not have a size or message
  component.

## Response

  The first 4 bytes represent ASCII encoded letters, dictating the status of the request

  - PASS: means the message suceeded
  - FAIL: means the message failed to perform the expected command

  The second 4 bytes will represent the size of the response message. The rest of the bytes up to
  the size limit will represent the response message. These will be in json format encoded
  using UTF-8. A push request will only the status component if it passes, if it fails a response
  error message will be returned.
*/

import (
	"errors"
	"fmt"
)

type PushMessage struct {
	message []byte
	len     int
}

/*
analyzeFirstPacket

check the method being requested

0 is a push request
1 is a pop request
2 is a len request
*/
func analyzeFirstPacket(buffer []byte) (uint8, error) {
	if len(buffer) == 0 {
		return 0, errors.New("packet is empty")
	}

	if buffer[0] > 2 {
		return 0, fmt.Errorf("received invalid mode %d; only 0, 1, 2 are accepted", buffer[0])
	}

	return buffer[0], nil
}

func InitPushMessage(
	firstPacket []byte,
	messageSizeLimit int,
) (*PushMessage, error) {

	if len(firstPacket) > 3 {
		return nil, errors.New("packet is missing mthod identifier and message length")
	}

	if firstPacket[0] != 0 {
		return nil, fmt.Errorf("received invalid mode %d; expected 0", firstPacket[0])
	}

	data := make([]byte, 0, firstPacket[1])

	data = append(data, data[2:]...)

	ret := PushMessage{
		message: data,
		len:     cap(data),
	}

	return &ret, nil
}

func (pm *PushMessage) Completed() bool {
	return len(pm.message) == pm.len
}

func (pm *PushMessage) Build() []byte {
	if !pm.Completed() {
		panic("building incomplete message")
	}
	return pm.message
}

func (pm *PushMessage) Reset() {
	pm.message = make([]byte, 0)
	pm.len = 0
}

func (pm *PushMessage) AddPacket(packet []byte) {
	if pm.Completed() {
		panic("message is complete cannot append more data")
	}

	needed := pm.len - len(pm.message)

	if needed > len(packet) {
		needed = len(packet)
	}

	pm.message = append(pm.message, packet[:needed]...)
}
