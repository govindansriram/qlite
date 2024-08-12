package server

import (
	"bytes"
	"context"
	"sync"
	"testing"
	"time"
)

func TestQueue_processor(t *testing.T) {

	t.Run("Test removeContext by duration", func(t *testing.T) {
		q := NewMSQueue(1_000, time.Second*10)
		_, err := q.Push([]byte{'a'})

		if err != nil {
			t.Fatal(err)
		}

		data, uid, err := q.Hide(time.Second * 0)

		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, []byte{'a'}) {
			t.Fatal("popped data is not the same")
		}

		remChan := make(chan bool)
		ctx, cf := context.WithTimeout(context.Background(), time.Second*2)

		defer cf()

		stay := true

		for stay {
			go func() {
				_, ok := q.processing.Load(uid)
				remChan <- ok
			}()

			select {
			case <-ctx.Done():
				state := <-remChan
				if state == false {
					stay = false
				}

				t.Fatal("failed to remove context fast enough")
			case state := <-remChan:
				if state == false {
					stay = false
				}
			}
		}

		data, err = q.queue.Dequeue()

		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(data, []byte{'a'}) {
			t.Error("data did not reappear")
		}

		pctx, cf := NewPopContext([]byte{'b'}, time.Second*0)

		if pctx.id == uid {
			t.Fatal("new context was not received")
		}

		defer cf()
	})

	t.Run("Test removeContext by cancellation", func(t *testing.T) {
		q := NewMSQueue(1_000, time.Second*10)
		_, err := q.Push([]byte{'a'})

		if err != nil {
			t.Fatal(err)
		}

		data, uid, err := q.Hide(time.Second * 2)

		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, []byte{'a'}) {
			t.Fatal("popped data is not the same")
		}

		err = q.cancel(uid)

		if err != nil {
			t.Error(err)
		}

		remChan := make(chan bool)
		ctx, cf := context.WithTimeout(context.Background(), time.Second*2)

		defer cf()

		stay := true

		for stay {
			go func() {
				_, ok := q.processing.Load(uid)
				remChan <- ok
			}()

			select {
			case <-ctx.Done():
				state := <-remChan
				if state == false {
					stay = false
				}

				t.Fatal("failed to remove context fast enough")
			case state := <-remChan:
				if state == false {
					stay = false
				}
			}
		}

		data, err = q.queue.Dequeue()

		if err != nil {
			t.Error(err)
		}

		if !bytes.Equal(data, []byte{'a'}) {
			t.Error("data did not reappear")
		}

		pctx, cf := NewPopContext([]byte{'b'}, time.Second*0)

		if pctx.id == uid {
			t.Fatal("new context was not received")
		}

		defer cf()
	})

	t.Run("Test removeContext by cancellation", func(t *testing.T) {
		q := NewMSQueue(1_000, time.Second*10)
		_, err := q.Push([]byte{'a'})

		if err != nil {
			t.Fatal(err)
		}

		data, uid, err := q.Hide(time.Second * 2)

		if err != nil {
			t.Fatal(err)
		}

		if !bytes.Equal(data, []byte{'a'}) {
			t.Fatal("popped data is not the same")
		}

		err = q.Delete(uid)

		if err != nil {
			t.Error(err)
		}

		remChan := make(chan bool)
		ctx, cf := context.WithTimeout(context.Background(), time.Second*2)

		defer cf()

		stay := true

		for stay {
			go func() {
				_, ok := q.processing.Load(uid)
				remChan <- ok
			}()

			select {
			case <-ctx.Done():
				state := <-remChan
				if state == false {
					stay = false
				}

				t.Fatal("failed to remove context fast enough")
			case state := <-remChan:
				if state == false {
					stay = false
				}
			}
		}

		data, err = q.queue.Dequeue()

		if err == nil {
			t.Error("dequeued empty queue")
		}
	})

	t.Run("Test concurrent read writes", func(t *testing.T) {
		q := NewMSQueue(1_000, time.Second*10)

		wg := sync.WaitGroup{}

		for range 100 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := q.Push([]byte{'a', 'b', 'c'})
				if err != nil {
					t.Error(err)
				}
			}()
		}

		wg.Wait()

		if q.Len() != 100 {
			t.Fatal("failed to add all values")
		}

		for idx := range 100 {
			wg.Add(1)

			go func() {
				defer wg.Done()
				_, uid, err := q.Hide(time.Second * 3)
				if err != nil {
					t.Error(err)
				}
				if idx%2 == 0 {
					err = q.cancel(uid)
					if err != nil {
						t.Error(err)
					}
				} else {
					err := q.Delete(uid)
					if err != nil {
						t.Error(err)
					}
				}
			}()
		}

		wg.Wait()

		cancelContext, cf := context.WithTimeout(context.Background(), time.Second*2)
		defer cf()

		passedChan := make(chan struct{})

		alive := true

		go func() {
			for q.Len() != 50 && alive {
			}
			if !alive {
				return
			}
			passedChan <- struct{}{}
		}()

		select {
		case <-cancelContext.Done():
			alive = false
			t.Fatal("failed to reach appropriate size")

		case <-passedChan:
			return
		}
	})
}
