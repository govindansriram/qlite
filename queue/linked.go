package queue

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"
	"sync/atomic"
	"time"
)

type message struct {
	// a linked list of messages
	buffer []byte   // the message buffer
	next   *message // the following message in the linked list
}

type LinkedQueue struct {
	// a queue that is backed by a linked list. LinkedQueue tries its best to maintain
	// ordering and ensures every message is consumed at least once. This is made possible through
	// hidden messages. Hidden messages are not deleted they are just hidden from
	// consumers. They are deleted when a consumer confirms the message has
	// been processed. This ensures that if a consumer fails to process a message it is not lost.
	// If a hidden message is not deleted in a set amount of time, it is reinstated back into the queue.

	length         atomic.Int32  // the length of the queue
	cap            uint32        // how many nodes are allowed on the queue
	maxMessageSize uint32        // max size of the message buffer
	maxHiddenTime  time.Duration // the max duration a message can be hidden for
	head           *message      // the current earliest added message
	tail           *message      // the last message in the queue, new values in the queue become the tail
	lock           sync.Mutex    // synchronizes queue alterations
	processing     sync.Map      // all the messages being processed by a consumer
	killCh         chan struct{} // stops the processor from running
}

/*
NewLinkedQueue

this is the recommended way to get a LinkedQueue, it initializes the queue
with a background processor, that cleans up hidden messages
*/
func NewLinkedQueue(cap, messageSizeLimit uint32, maxHiddenTime time.Duration) *LinkedQueue {
	lq := LinkedQueue{
		cap:            cap,
		maxMessageSize: messageSizeLimit,
		killCh:         make(chan struct{}),
		maxHiddenTime:  maxHiddenTime,
	}

	go lq.processor()
	return &lq
}

/*
Kill

this ends the processor, and the queue should not be used after this point
*/
func (l *LinkedQueue) Kill() {
	l.killCh <- struct{}{}
}

/*
Len

returns how many messages are enqueued
*/
func (l *LinkedQueue) Len() int32 {
	return l.length.Load()
}

/*
processor

processor ensures all messages that have not been removed before their deadline are
enqueued once again, and permanently deletes removed messages
*/
func (l *LinkedQueue) processor() {
	for {
		select {
		case <-l.killCh:
			return
		default:
			l.processing.Range(func(key, value any) bool {
				uid, ok := key.(uuid.UUID)
				if !ok {
					panic("could not cast to uuid")
				}

				ctx, ok := key.(*popContext)
				if !ok {
					panic("could not cast to popContext")
				}

				select {
				case <-ctx.Done():
					l.processing.Delete(uid)
					l.reappear(ctx.node)
					deletePopContext(ctx)
				case <-ctx.Removed():
					l.processing.Delete(uid)
					deletePopContext(ctx)
				default:
				}
				return true
			})
		}
	}
}

/*
Push

enqueues data at the end of the queue
*/
func (l *LinkedQueue) Push(buffer []byte) (uint32, error) {
	if len(buffer) == 0 {
		return 0, errors.New("pushing empty buffer")
	}

	if uint32(len(buffer)) > l.maxMessageSize {
		return 0, errors.New("message exceeds acceptable message size")
	}

	mess := message{
		buffer: buffer,
		next:   nil,
	} // maybe create sync pool

	l.lock.Lock()
	defer l.lock.Unlock()

	if uint32(l.Len()) == l.cap {
		return 0, errors.New("queue is full")
	}

	l.length.Add(1)

	if l.tail == nil {
		l.head = &mess
		l.tail = &mess
		return 1, nil
	}

	l.tail.next = &mess
	l.tail = &mess

	return uint32(l.Len()), nil
}

/*
Hide

removes the first message from the queue and sends it to the processor
*/
func (l *LinkedQueue) Hide(unlinkTime time.Duration) ([]byte, uuid.UUID, error) {
	if l.maxHiddenTime < unlinkTime {
		return nil, [16]byte{}, errors.New("provided hidden duration is longer then the max viable time")
	}

	l.lock.Lock()

	if uint32(l.Len()) == 0 {
		l.lock.Unlock()
		return nil, [16]byte{}, errors.New("queue is full")
	}

	popped := l.head
	l.head = popped.next
	l.lock.Unlock()

	popCtx, _ := newPopContext(popped, unlinkTime)
	l.processing.Store(popCtx.id, popCtx)

	return popped.buffer, popCtx.id, nil
}

/*
reappear

enqueues the hidden message at the end of the queue
*/
func (l *LinkedQueue) reappear(nd *message) {
	l.lock.Lock()
	defer l.lock.Unlock()

	nd.next = nil
	l.tail.next = nd
	l.tail = nd
}

/*
Delete

signals to the processor that the message has been processed
*/
func (l *LinkedQueue) Delete(id uuid.UUID) error {

	value, ok := l.processing.Load(id)

	if !ok {
		return errors.New("the key you provided does not correlate to any value being processed, " +
			"expiration time may have been exceeded")
	}

	ctx, ok := value.(*popContext)

	if !ok {
		panic("could not cast to popContext")
	}

	if err := ctx.Remove(); err != nil {
		return err
	}

	l.length.Add(-1)

	return nil
}

type popContext struct {
	// popContext is the context used by the processing job, it instructs the processor on how long to wait for
	// a job to complete

	id        uuid.UUID     // the id of the processing job
	removed   chan struct{} // if the data in the struct has been removed
	done      chan struct{} // if the request has finished
	startTime time.Time     // when the message has started processing
	duration  time.Duration // the max time processing can occur for
	lock      sync.Mutex    // synchronizes channel handling
	signal    bool          // becomes true when one of the two channels have been written too
	errReason error         // why the context ended
	node      *message      // the node being processed
}

/*
Reset

resets a context, so it can be reused (reduces gc calls)
*/
func (p *popContext) Reset(nd *message, duration time.Duration) context.CancelFunc {
	uid, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}

	p.id = uid
	p.signal = false
	p.errReason = nil
	p.node = nd
	p.duration = duration
	p.startTime = time.Now()

	return p.countDown()
}

/*
countDown

starts a time limit, if the time limit is met before the removed channel
has been written too, the done channel will be written too. This ensures only
one channel can be written too at once. This also returns a CancelFunc which will write to
the done channel prematurely if possible.
*/
func (p *popContext) countDown() context.CancelFunc {
	go func() {
		endTime, _ := p.Deadline()
		for endTime.After(time.Now()) {
			p.lock.Lock()
			if p.signal {
				p.lock.Unlock()
				return
			}
			p.lock.Unlock()
		}

		p.lock.Lock()
		if !p.signal {
			p.signal = true
			p.lock.Unlock()
			p.done <- struct{}{}
			p.errReason = context.DeadlineExceeded
			return
		}
		p.lock.Unlock()
	}()

	return func() {
		p.lock.Lock()
		if !p.signal {
			p.signal = true
			p.lock.Unlock()
			p.done <- struct{}{}
			p.errReason = context.Canceled
			return
		}
		p.lock.Unlock()
	}
}

/*
Deadline

returns the time when the ctx deadline will be hit, ok will always be true
*/
func (p *popContext) Deadline() (deadline time.Time, ok bool) {
	return p.startTime.Add(p.duration), true
}

/*
Done

returns the done channel, this signals the deadline has been hit
*/
func (p *popContext) Done() <-chan struct{} {
	return p.done
}

/*
Removed

returns the Removed channel, this signals the message should be deleted
*/
func (p *popContext) Removed() <-chan struct{} {
	return p.removed
}

/*
Remove

rights to the removed channel if possible
*/
func (p *popContext) Remove() error {
	p.lock.Lock()
	if !p.signal {
		p.signal = true
		p.lock.Unlock()
		p.errReason = errors.New("request has finished")
		p.removed <- struct{}{}
		return nil
	}
	p.lock.Unlock()

	return p.Err()
}

/*
Err

the reason why the context was cancelled if it was cancelled
*/
func (p *popContext) Err() error {
	return p.errReason
}

func (p *popContext) Value(key any) any {
	_ = key
	return nil
}

func (p *popContext) Message() *message {
	return p.node
}

var contextPool = sync.Pool{
	New: func() any {
		pc := popContext{
			removed: make(chan struct{}),
			done:    make(chan struct{}),
		}
		return &pc
	},
}

/*
newPopContext

gets a new context from the sync pool
*/
func newPopContext(nd *message, duration time.Duration) (*popContext, context.CancelFunc) {
	val := contextPool.Get()
	if ctx, ok := val.(*popContext); ok {
		cf := ctx.Reset(nd, duration)
		return ctx, cf
	} else {
		panic("could not cast to context")
	}
}

/*
deletePopContext

adds a context to the sync pool
*/
func deletePopContext(ctx *popContext) {
	contextPool.Put(ctx)
}
