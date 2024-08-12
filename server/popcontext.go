package server

import (
	"context"
	"errors"
	"github.com/google/uuid"
	"sync"
	"time"
)

type PopContext struct {
	// PopContext is the context used by the processing job, it instructs the processor on how long to wait for
	// a job to complete

	id        uuid.UUID     // the id of the processing job
	removed   chan struct{} // if the data in the struct has been removed
	done      chan struct{} // if the request has finished
	startTime time.Time     // when the message has started processing
	duration  time.Duration // the max time processing can occur for
	lock      sync.Mutex    // synchronizes channel handling
	signal    bool          // becomes true when one of the two channels have been written too
	errReason error         // why the context ended
	data      []byte        // the data being processed
}

/*
Reset

resets a context, so it can be reused (reduces gc calls)
*/
func (p *PopContext) Reset(buffer []byte, duration time.Duration) context.CancelFunc {
	uid, err := uuid.NewUUID()
	if err != nil {
		panic(err)
	}

	p.id = uid
	p.signal = false
	p.errReason = nil
	p.data = buffer
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
func (p *PopContext) countDown() context.CancelFunc {
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
			p.errReason = context.DeadlineExceeded
			p.done <- struct{}{}
			return
		}
		p.lock.Unlock()
	}()

	return func() {
		p.lock.Lock()
		if !p.signal {
			p.signal = true
			p.lock.Unlock()
			p.errReason = context.Canceled
			p.done <- struct{}{}
			return
		}
		p.lock.Unlock()
	}
}

/*
Deadline

returns the time when the ctx deadline will be hit, ok will always be true
*/
func (p *PopContext) Deadline() (deadline time.Time, ok bool) {
	return p.startTime.Add(p.duration), true
}

/*
Done

returns the done channel, this signals the deadline has been hit
*/
func (p *PopContext) Done() <-chan struct{} {
	return p.done
}

/*
Removed

returns the Removed channel, this signals the message should be deleted
*/
func (p *PopContext) Removed() <-chan struct{} {
	return p.removed
}

/*
Remove

rights to the removed channel if possible
*/
func (p *PopContext) Remove() error {
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
func (p *PopContext) Err() error {
	return p.errReason
}

func (p *PopContext) Value(key any) any {
	_ = key
	return nil
}

func (p *PopContext) Message() []byte {
	return p.data
}

var contextPool = sync.Pool{
	New: func() any {
		pc := PopContext{
			removed: make(chan struct{}, 1),
			done:    make(chan struct{}, 1),
		}
		return &pc
	},
}

/*
NewPopContext

gets a new context from the sync pool
*/
func NewPopContext(data []byte, duration time.Duration) (*PopContext, context.CancelFunc) {
	val := contextPool.Get()
	if ctx, ok := val.(*PopContext); ok {
		cf := ctx.Reset(data, duration)
		return ctx, cf
	} else {
		panic("could not cast to context")
	}
}

/*
deletePopContext

adds a context to the sync pool
*/
func deletePopContext(ctx *PopContext) {
	contextPool.Put(ctx)
}
