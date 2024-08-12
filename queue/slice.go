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

type Slice struct {
	backingSlice [][]byte
	lock         sync.Mutex
	readPos      atomic.Int32
	writePos     atomic.Int32
}

/*
NewSliceQueue

creates a new Slice
*/
func NewSliceQueue(startSize int) Slice {
	bs := make([][]byte, startSize)
	return Slice{
		backingSlice: bs,
	}
}

/*
Len

gets the length of the queue
*/
func (s *Slice) Len() int32 {
	return s.writePos.Load() - s.readPos.Load()
}

/*
Dequeue

removes the first element from the Queue
*/
func (s *Slice) Dequeue() ([]byte, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if s.Len() == 0 {
		return nil, errors.New("cannot pop from empty slice")
	}

	readPos := s.readPos.Load()

	response := s.backingSlice[readPos]
	s.backingSlice[readPos] = []byte{} // mark the popped position as empty
	s.readPos.Add(1)

	return response, nil
}

/*
Enqueue

appends an element to the end of the Queue, returns the position of the element in the queue
*/
func (s *Slice) Enqueue(message []byte) int32 {
	if len(message) == 0 {
		panic("message is empty")
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	if s.writePos.Load() == int32(cap(s.backingSlice)) {
		s.adjust()
	}

	writePos := s.writePos.Load()

	if writePos == int32(cap(s.backingSlice)) {
		var dataSlice [][]byte
		dataSlice = make([][]byte, cap(s.backingSlice)*2)
		copy(dataSlice, s.backingSlice)
		dataSlice[writePos] = message
		s.backingSlice = dataSlice
	} else {
		s.backingSlice[writePos] = message
	}

	return s.writePos.Add(1)
}

/*
adjust

as Pop's occur the indexes that have been popped will be marked as empty. Since pops are sequential we can assume
that after some time there will be a continuous sequence of empty indexes starting from the beginning of the slice.

adjust ensures that values that have not been popped are shifted back to the front taking up those empty spaces.
*/
func (s *Slice) adjust() {

	var startingPos = -1
	index := 0

	for index < len(s.backingSlice) {
		messLen := len(s.backingSlice[index])

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
	for i := startingPos; i < len(s.backingSlice); i++ {
		s.backingSlice[index] = s.backingSlice[i]
		s.backingSlice[i] = []byte{}
		index++
	}

	s.writePos.Store(int32(index))
	s.readPos.Store(0)
}
