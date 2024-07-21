package queue

import (
	"bytes"
	"math"
	"math/rand/v2"
	"sync"
	"testing"
)

func generateBytes() []byte {
	length := rand.IntN(8) + 1
	buffer := make([]byte, length)

	for idx := range buffer {
		buffer[idx] = uint8(rand.IntN(math.MaxUint8))
	}

	return buffer
}

func TestQueue_Pop(t *testing.T) {
	const routines = 100

	fullBuffer := make([][]byte, routines)

	for idx := range routines {
		fullBuffer[idx] = generateBytes()
	}

	q := Queue{
		backingSlice:   fullBuffer,
		maxMessageSize: nil,
		maxMessages:    routines,
	}

	q.readPos.Store(0)
	q.writePos.Store(routines)

	wg := sync.WaitGroup{}

	for range routines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := q.Pop()

			if err != nil {
				t.Error(err)
			}
		}()
	}

	wg.Wait()

	if q.Len() != 0 {
		t.Error("queue is not empty")
	}
}

func TestQueue_Push(t *testing.T) {
	const routines = 50

	fullBuffer := make([][]byte, routines/10)

	q := Queue{
		backingSlice:   fullBuffer,
		maxMessageSize: nil,
		maxMessages:    routines,
	}

	q.readPos.Store(0)
	q.writePos.Store(0)

	wg := sync.WaitGroup{}

	byteSlice := make([][]byte, routines)

	for idx := range routines {
		byteData := generateBytes()
		byteSlice[idx] = byteData
		wg.Add(1)

		go func() {
			defer wg.Done()
			_, err := q.Push(byteData)
			if err != nil {
				t.Error(err)
			}
		}()
	}

	wg.Wait()

	if q.Len() != routines {
		t.Error("queue is not full")
	}

	if len(q.backingSlice) != routines {
		t.Error("queue is not full")
	}

	compareBytes(t, byteSlice, q.backingSlice)

}

func compareBytes(t *testing.T, control, test [][]byte) {
	for _, i := range control {
		found := false
		for _, j := range test {
			if bytes.Equal(i, j) {
				found = true
			}
		}

		if !found {
			t.Error("byte was not added")
		}
	}
}

/*
TestQueue_All

simulate 100 concurrent reads and 100 concurrent writes in random order
*/
func TestQueue_All(t *testing.T) {
	const mx = uint32(100)
	queue := NewQueue(mx, nil)

	wgPop := sync.WaitGroup{}
	dataChan := make(chan []byte)
	failedChan := make(chan struct{})

	popper := func() {
		data, err := queue.Pop()
		if err == nil {
			dataChan <- data
		} else {
			failedChan <- struct{}{}
		}
	}

	pusher := func(data []byte) {
		_, err := queue.Push(data)
		if err != nil {
			panic(err)
		}
	}

	var scheduledPop int
	var scheduledPush int

	opts := []int{-1, 1}

	control := make([][]byte, 0, mx)
	test := make([][]byte, 0, mx)

	for range mx * 2 {
		num := opts[rand.IntN(2)]

		if (num == -1 && scheduledPop == int(mx)) || (num == 1 && scheduledPush == int(mx)) {
			num *= -1
		}

		if num == -1 {
			wgPop.Add(1)
			go popper()
			scheduledPop++
		} else {
			byt := generateBytes()
			control = append(control, byt)
			go pusher(byt)
			scheduledPush++
		}
	}

	go func() {
		for {
			data := <-dataChan
			test = append(test, data)
			wgPop.Done()
		}
	}()

	go func() {
		wgPop.Wait()
		close(failedChan)
	}()

	for range failedChan {
		go popper()
	}

	if len(control) != len(test) {
		t.Error("collected arrays are not the same")
	}

	compareBytes(t, control, test)

	if queue.Len() != 0 {
		t.Error("queue is not empty")
	}
}
