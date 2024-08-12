package queue

import (
	"errors"
	"sync"
	"sync/atomic"
)

type message struct {
	// a linked list of messages
	buffer []byte   // the message buffer
	next   *message // the following message in the linked list
}

type LinkedQueue struct {
	length atomic.Int32 // the length of the queue
	head   *message     // the current earliest added message
	tail   *message     // the last message in the queue, new values in the queue become the tail
	lock   sync.Mutex   // synchronizes queue alterations
}

/*
Len

returns how many messages are enqueued
*/
func (l *LinkedQueue) Len() int32 {
	return l.length.Load()
}

/*
Enqueue

enqueues data at the end of the queue
*/
func (l *LinkedQueue) Enqueue(buffer []byte) int32 {

	mess := message{
		buffer: buffer,
		next:   nil,
	} // maybe create sync pool

	l.lock.Lock()
	defer l.lock.Unlock()

	l.length.Add(1)

	if l.tail == nil {
		l.head = &mess
		l.tail = &mess
		return 1
	}

	l.tail.next = &mess
	l.tail = &mess

	return l.Len()
}

/*
Dequeue

removes the first message from the queue and sends it to the processor
*/
func (l *LinkedQueue) Dequeue() ([]byte, error) {

	l.lock.Lock()

	if l.Len() == 0 {
		l.lock.Unlock()
		return nil, errors.New("queue is empty")
	}

	if l.head == nil || l.tail == nil {
		l.lock.Unlock()
		return nil, errors.New("queue is empty")
	}

	defer l.length.Add(-1)

	popped := l.head

	l.head = popped.next
	l.lock.Unlock()

	return popped.buffer, nil
}
