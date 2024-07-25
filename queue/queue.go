/*
Author: Sriram Govindan
Date: 2024-07-21
Description: a fast in memory queue that is thread safe.

How does it work?

a queue is backed by a slice, to increase speed we never append to a slice we only assign (s[1] = 1). Because you can
both push and pop from a slice we need to keep track of the current write and read position. If we require more space,
we create a new slice with twice the capacity and copy over the data.
*/

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
	maxMessageSize uint32
	readPos        atomic.Int32
	writePos       atomic.Int32
}

/*
isNil

checks if the backing slice of a queue is nil (forces users to use the NewQueue method)
*/
func isNil(slice [][]byte) {
	if slice == nil {
		panic("backingSlice cannot be nil, please use NewQueue method to create a queue")
	}
}

/*
NewQueue

creates a new Queue
*/
func NewQueue(maxMessCount uint32, maxMessSize uint32) Queue {
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

/*
Len

gets the length of the queue
*/
func (q *Queue) Len() int32 {
	return q.writePos.Load() - q.readPos.Load()
}

/*
Pop

removes the first element from the Queue
*/
func (q *Queue) Pop() ([]byte, error) {
	isNil(q.backingSlice)
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.Len() == 0 {
		return nil, errors.New("cannot pop from empty slice")
	}

	readPos := q.readPos.Load()

	response := q.backingSlice[readPos]
	q.backingSlice[readPos] = []byte{} // mark the popped position as empty
	q.readPos.Add(1)

	return response, nil
}

/*
Push

appends an element to the end of the Queue, returns the position of the element in the queue
*/
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

	if len(message) > int(q.maxMessageSize) {
		return 0, errors.New("message is too large")
	}

	if q.writePos.Load() == int32(cap(q.backingSlice)) {
		q.adjust()
	}

	writePos := q.writePos.Load()

	if writePos == int32(cap(q.backingSlice)) {
		var dataSlice [][]byte

		if int(q.maxMessages/2) < cap(q.backingSlice) {
			// double the capacity if  double the current capacity is under the max message limit
			dataSlice = make([][]byte, cap(q.backingSlice)*2)
		} else {
			// set the capacity to the max
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

/*
adjust

as Pop's occur the indexes that have been popped will be marked as empty. Since pops are sequential we can assume
that after some time there will be a continuous sequence of empty indexes starting from the beginning of the slice.

adjust ensures that values that have not been popped are shifted back to the front taking up those empty spaces.
*/
func (q *Queue) adjust() {
	isNil(q.backingSlice)

	var startingPos = -1
	index := 0

	for index < len(q.backingSlice) {
		messLen := len(q.backingSlice[index])

		if index == 0 && messLen > 0 {
			// there are no empty spaces in the slice
			return
		}

		if messLen > 0 {
			startingPos = index
			break
		}

		index++
	}

	if startingPos == -1 {
		// entire slice is empty
		startingPos = index
	}

	index = 0
	// shift non popped data back to the front
	for i := startingPos; i < len(q.backingSlice); i++ {
		q.backingSlice[index] = q.backingSlice[i]
		q.backingSlice[i] = []byte{}
		index++
	}

	q.writePos.Store(int32(index))
	q.readPos.Store(0)
}
