package queue

import (
	"errors"
	"sync"
	"sync/atomic"
)

type Queue struct {
	backingSlice   [][]byte
	lock           sync.Mutex
	maxMessages    uint32
	maxMessageSize *uint32
	readPos        atomic.Int32
	writePos       atomic.Int32
}

func isNil(slice [][]byte) {
	if slice == nil {
		panic("backingSlice cannot be nil, please use NewQueue method to create a queue")
	}
}

func NewQueue(maxMessCount uint32, maxMessSize *uint32) Queue {
	var split = maxMessCount

	if maxMessCount > 10 {
		split = maxMessCount / 10
	}

	bs := make([][]byte, split)

	return Queue{
		backingSlice:   bs,
		maxMessages:    maxMessCount,
		maxMessageSize: maxMessSize,
	}
}

func (q *Queue) Len() int32 {
	return q.writePos.Load() - q.readPos.Load()
}

func (q *Queue) Pop() ([]byte, error) {
	isNil(q.backingSlice)
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.Len() == 0 {
		return nil, errors.New("cannot pop from empty slice")
	}

	readPos := q.readPos.Load()

	response := q.backingSlice[readPos]
	q.backingSlice[readPos] = []byte{}
	q.readPos.Add(1)

	return response, nil
}

func (q *Queue) Push(message []byte) (uint32, error) {
	isNil(q.backingSlice)

	if len(message) == 0 {
		panic("message is empty")
	}

	q.lock.Lock()
	defer q.lock.Unlock()

	if q.Len() >= int32(q.maxMessages) {
		return 0, errors.New("queue is full")
	}

	if q.maxMessageSize != nil && len(message) > int(*q.maxMessageSize) {
		return 0, errors.New("message is too large")
	}

	if q.writePos.Load() == int32(cap(q.backingSlice)) {
		q.adjust()
	}

	writePos := q.writePos.Load()

	if writePos == int32(cap(q.backingSlice)) {
		var dataSlice [][]byte

		if int(q.maxMessages/2) < cap(q.backingSlice) {
			dataSlice = make([][]byte, cap(q.backingSlice)*2)
		} else {
			dataSlice = make([][]byte, q.maxMessages)
		}

		copy(dataSlice, q.backingSlice)
		dataSlice[writePos] = message
		q.backingSlice = dataSlice
	} else {
		q.backingSlice[writePos] = message
	}

	q.writePos.Add(1)
	return uint32(q.Len()), nil
}

func (q *Queue) adjust() {
	isNil(q.backingSlice)

	var startingPos = -1
	index := 0

	for startingPos < 0 || index < len(q.backingSlice) {
		messLen := len(q.backingSlice[index])

		if index == 0 && messLen >= 0 {
			return
		}

		if messLen >= 0 {
			startingPos = index
		}

		index++
	}

	index = 0
	for i := startingPos; i < len(q.backingSlice); i++ {
		q.backingSlice[index] = q.backingSlice[i]
		q.backingSlice[i] = []byte{}
		index++
	}

	q.writePos.Store(int32(index))
	q.readPos.Store(0)
}
