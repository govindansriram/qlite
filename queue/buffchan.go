package queue

import (
	"context"
	"sync/atomic"
)

type BuffChan struct {
	queue chan []byte
	len   atomic.Int32
}

func NewBuffChan() *BuffChan {
	return &BuffChan{
		queue: make(chan []byte, 1_000_000),
	}
}

func (bc *BuffChan) Enqueue(val []byte) int32 {
	select {
	case bc.queue <- val:
		return bc.len.Add(1)
	default:
		panic("queue is full")
	}
}

func (bc *BuffChan) Dequeue() ([]byte, error) {
	select {
	case val := <-bc.queue:
		return val, nil
	default:
		return nil, context.DeadlineExceeded
	}
}

func (bc *BuffChan) Len() int32 {
	return bc.len.Load()
}
