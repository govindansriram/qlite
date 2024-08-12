package queue

import (
	"errors"
	"sync/atomic"
)

type node struct {
	value []byte
	next  atomic.Pointer[node]
}

func newNode(value []byte) node {
	return node{
		value: value,
	}
}

type MichaelScott struct {
	head atomic.Pointer[node]
	tail atomic.Pointer[node]
	len  atomic.Int32
}

func NewMichaelScott() *MichaelScott {
	nod := newNode(nil)
	msq := MichaelScott{}
	msq.head.Store(&nod)
	msq.tail.Store(&nod)
	return &msq
}

func (msq *MichaelScott) Enqueue(val []byte) int32 {
	nd := newNode(val)

	for {
		tail := msq.tail.Load()
		next := tail.next.Load()

		if tail == msq.tail.Load() {
			if next == nil {
				if tail.next.CompareAndSwap(next, &nd) {
					msq.tail.CompareAndSwap(tail, &nd)
					return msq.len.Add(1)
				}
			} else {
				msq.tail.CompareAndSwap(tail, next)
			}
		}
	}
}

func (msq *MichaelScott) Dequeue() ([]byte, error) {
	for {
		tail := msq.tail.Load()
		head := msq.head.Load()
		next := head.next.Load()

		if head == msq.head.Load() {
			if head == tail {
				if next == nil {
					return nil, errors.New("queue is empty")
				}
				msq.tail.CompareAndSwap(tail, next)
			} else {
				ret := next.value
				if msq.head.CompareAndSwap(head, next) {
					msq.len.Add(-1)
					return ret, nil
				}
			}
		}
	}
}

func (msq *MichaelScott) Len() int32 {
	return msq.len.Load()
}
