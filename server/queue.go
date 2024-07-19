package server

import (
	"errors"
	"sync"
)

type Queue struct {
	backingSlice   [][]byte
	lock           sync.Mutex
	maxMessages    *uint32
	maxMessageSize *uint32
}

type emptyError struct {
	err error
}

func (e emptyError) Error() string {
	return "queue is empty"
}

func (q *Queue) Pop() ([]byte, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.Len() == 0 {
		return nil, emptyError{}
	}

	response := q.backingSlice[0]

	offByOne := make([][]byte, cap(q.backingSlice))
	copy(offByOne, q.backingSlice[1:])
	q.backingSlice = offByOne

	return response, nil
}

func (q *Queue) Push(message []byte) error {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.maxMessages != nil && q.Len() >= int(*q.maxMessages) {
		return errors.New("queue is full")
	}

	if q.maxMessageSize != nil && len(message) >= int(*q.maxMessageSize) {
		return errors.New("message is too large")
	}

	q.backingSlice = append(q.backingSlice, message)

	return nil
}

func (q *Queue) Len() int {
	return len(q.backingSlice)
}
