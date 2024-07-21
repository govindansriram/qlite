package queue

import (
	"errors"
	"sync"
)

type Queue struct {
	backingSlice   [][]byte
	lock           sync.Mutex
	maxMessages    uint32
	maxMessageSize *uint32
}

func Init(maxMessCount uint32, maxMessSize *uint32) Queue {
	var split = maxMessCount

	if maxMessCount > 3 {
		split = maxMessCount / 3
	}

	bs := make([][]byte, 0, split)

	return Queue{
		backingSlice:   bs,
		maxMessages:    maxMessCount,
		maxMessageSize: maxMessSize,
	}
}

func (q *Queue) Len() int {

	q.lock.Lock()
	defer q.lock.Unlock()

	return len(q.backingSlice)
}

func (q *Queue) Pop() ([]byte, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if len(q.backingSlice) == 0 {
		return nil, errors.New("cannot pop from empty slice")
	}

	response := q.backingSlice[0]

	offByOne := make([][]byte, len(q.backingSlice)-1, cap(q.backingSlice))
	copy(offByOne, q.backingSlice[1:])
	q.backingSlice = offByOne

	return response, nil
}

func (q *Queue) Push(message []byte) (uint32, error) {
	q.lock.Lock()
	defer q.lock.Unlock()

	if len(q.backingSlice) >= int(q.maxMessages) {
		return 0, errors.New("queue is full")
	}

	if q.maxMessageSize != nil && len(message) > int(*q.maxMessageSize) {
		return 0, errors.New("message is too large")
	}

	q.backingSlice = append(q.backingSlice, message)

	return uint32(len(q.backingSlice)), nil
}
