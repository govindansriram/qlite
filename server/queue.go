package server

import (
	queue2 "benchai/qlite/queue"
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"
	"time"
)

type qu interface {
	Len() int32
	Dequeue() ([]byte, error)
	Enqueue(val []byte) int32
}

type Queue struct {
	maxMessageSize uint32        // max size of the message buffer
	maxHiddenTime  time.Duration // the max duration a message can be hidden for
	processing     sync.Map      // all the messages being processed by a consumer
	killCh         chan struct{} // stops the processor from running
	queue          qu            // underlying queue datastructure
	cancelMap      sync.Map      // a map containing all the context cancel funcs
}

/*
processor

processor ensures all messages that have not been removed before their deadline are
enqueued once again, and permanently deletes removed messages
*/
func (q *Queue) processor() {
	for {
		select {
		case <-q.killCh:
			return
		default:
			q.processing.Range(func(key, value any) bool {
				uid, ok := key.(uuid.UUID)
				if !ok {
					panic("could not cast to uuid")
				}

				ctx, ok := value.(*PopContext)
				if !ok {
					panic("could not cast to PopContext")
				}

				select {
				case <-ctx.Done():
					q.reappear(ctx.Message())
					q.processing.Delete(uid)
					q.cancelMap.Delete(uid)
					deletePopContext(ctx)
				case <-ctx.Removed():
					q.processing.Delete(uid)
					q.cancelMap.Delete(uid)
					deletePopContext(ctx)
				default:
				}
				return true
			})
		}
	}
}

/*
Kill

this ends the processor, and the qu should not be used after this point
*/
func (q *Queue) Kill() {
	close(q.killCh)
}

/*
reappear

enqueues the hidden message at the end of the qu
*/
func (q *Queue) reappear(mess []byte) {
	_ = q.queue.Enqueue(mess)
}

/*
Len

returns how many messages are enqueued
*/
func (q *Queue) Len() int32 {
	return q.queue.Len()
}

/*
Push

enqueues data at the end of the qu
*/
func (q *Queue) Push(buffer []byte) (int32, error) {
	if len(buffer) == 0 {
		return -1, errors.New("pushing empty buffer")
	}

	if uint32(len(buffer)) > q.maxMessageSize {
		return -1, errors.New("message exceeds acceptable message size")
	}

	pos := q.queue.Enqueue(buffer)
	return pos, nil
}

/*
Hide

removes the first message from the qu and sends it to the processor
*/
func (q *Queue) Hide(unlinkTime time.Duration) ([]byte, uuid.UUID, error) {
	if q.maxHiddenTime < unlinkTime {
		return nil, [16]byte{}, errors.New("provided hidden duration is longer then the max viable time")
	}

	data, err := q.queue.Dequeue()

	if err != nil {
		return nil, [16]byte{}, err
	}

	popCtx, cf := NewPopContext(data, unlinkTime) // use cf

	q.processing.Store(popCtx.id, popCtx)
	q.cancelMap.Store(popCtx.id, cf)
	return data, popCtx.id, nil
}

/*
Delete

signals to the processor that the message has been processed
*/
func (q *Queue) Delete(id uuid.UUID) error {

	value, ok := q.processing.Load(id)

	if !ok {
		return errors.New("the key you provided does not correlate to any value being processed, " +
			"expiration time may have been exceeded")
	}

	ctx, ok := value.(*PopContext)

	if !ok {
		panic("could not cast to PopContext")
	}

	if err := ctx.Remove(); err != nil {
		return err
	}

	return nil
}

/*
Cancel

cancels the popContext
*/
func (q *Queue) cancel(id uuid.UUID) error {
	val, ok := q.cancelMap.Load(id)
	if !ok {
		return errors.New("id is not being processed")
	}

	cf, ok := val.(context.CancelFunc)

	if !ok {
		panic("could not cast to cancel function")
	}

	cf()

	return nil
}

/*
NewMSQueue

A qu that uses the Michael Scott Lockless qu algorithm
*/
func NewMSQueue(maxMessageSize uint32, maxHiddenTime time.Duration) *Queue {
	q := queue2.NewMichaelScott()
	qu := Queue{
		maxMessageSize: maxMessageSize,
		maxHiddenTime:  maxHiddenTime,
		killCh:         make(chan struct{}),
		queue:          q,
	}

	go qu.processor()
	return &qu
}
