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
	readPos        atomic.Int32 // the index that can be popped
	unstagedPos    atomic.Int32
	/*
		everything between the readPos and unstagedPos is a pop that has not been
		commited
	*/
	writePos atomic.Int32 // the index which can be written too
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

the amount of data in the queue including unstaged data
*/
func (q *Queue) Len() int32 {
	return q.writePos.Load() - q.unstagedPos.Load()
}

/*
len

the amount of staged data in the queue
*/
func (q *Queue) len() int32 {
	return q.writePos.Load() - q.unstagedPos.Load()
}

/*
StartPop

removes the first element from the Queue
*/
func (q *Queue) StartPop() ([]byte, error) {
	isNil(q.backingSlice)
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.len() == 0 {
		return nil, errors.New("cannot pop from empty slice")
	}

	readPos := q.readPos.Load()

	response := q.backingSlice[readPos]
	q.backingSlice[readPos] = []byte{} // mark the popped position as empty
	q.readPos.Add(1)

	return response, nil
}

/*
CommitPop

dequeues the data from the queue if the popped data was received
*/
func (q *Queue) CommitPop() {
	isNil(q.backingSlice)
	q.lock.Lock()
	defer q.lock.Unlock()
	q.unstagedPos.Add(1)
}

/*
RollbackPop

appends the popped data to the end of the queue if the client never received the popped data
*/
func (q *Queue) RollbackPop(message []byte) {
	isNil(q.backingSlice)
	q.lock.Lock()
	defer q.lock.Unlock()
	q.unstagedPos.Add(1)

	_, err := q.push(message)

	if err != nil {
		panic(err)
	}
}

func (q *Queue) push(message []byte) (uint32, error) {
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

	return q.push(message)
}

/*
adjust

as Pop's occur the indexes that have been popped will be marked as empty. Since pops are sequential we can assume
that after some time there will be a continuous sequence of empty indexes starting from the beginning of the slice.

adjust ensures that values that have not been popped are shifted back to the front taking up those empty spaces.
*/
func (q *Queue) adjust() {
	isNil(q.backingSlice)

	unstagedPos := q.unstagedPos.Load()
	readPos := q.readPos.Load()

	if unstagedPos == int32(len(q.backingSlice)) { // queue is completely empty
		q.unstagedPos.Store(0)
		q.writePos.Store(0)
		q.readPos.Store(0)
	}

	if readPos == 0 { // queue is completely full
		return
	}

	index := 0
	// shift non popped data back to the front
	for i := unstagedPos; i < int32(len(q.backingSlice)); i++ {
		q.backingSlice[index] = q.backingSlice[i]
		q.backingSlice[i] = []byte{}
		index++
	}

	q.writePos.Store(int32(index))
	q.unstagedPos.Store(0)
	q.writePos.Store(readPos - unstagedPos)
}
