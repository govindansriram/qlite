package queue

import (
	"bytes"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"testing"
)

const KB uint32 = 1024
const MB = KB * KB

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

	t.Run("concurrent pops", func(t *testing.T) {
		fullBuffer := make([][]byte, routines)

		for idx := range routines {
			fullBuffer[idx] = generateBytes()
		}

		q := Queue{
			backingSlice:   fullBuffer,
			maxMessageSize: 9 * MB,
			maxMessages:    routines,
		}

		q.readPos.Store(0)
		q.writePos.Store(routines)

		wg := sync.WaitGroup{}

		for range routines {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := q.StartPop()

				if err != nil {
					t.Error(err)
				}
			}()
		}

		wg.Wait()

		if q.Len() != 0 {
			t.Error("queue is not empty")
		}
	})

	t.Run("pop from empty queue", func(t *testing.T) {
		fullBuffer := make([][]byte, routines)

		q := Queue{
			backingSlice:   fullBuffer,
			maxMessageSize: 9 * MB,
			maxMessages:    routines,
		}

		q.readPos.Store(0)
		q.writePos.Store(0)

		if _, err := q.StartPop(); err == nil {
			t.Error("popped from empty queue")
		}
	})
}

func TestQueue_Push(t *testing.T) {
	const routines = 50

	t.Run("concurrent push", func(t *testing.T) {
		fullBuffer := make([][]byte, routines/10)

		q := Queue{
			backingSlice:   fullBuffer,
			maxMessageSize: 9 * MB,
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

		if cap(q.backingSlice) != routines {
			t.Error("capacity does not match")
		}
	})

	t.Run("push to full queue", func(t *testing.T) {

		fullBuffer := make([][]byte, routines)

		q := Queue{
			backingSlice:   fullBuffer,
			maxMessageSize: 9 * MB,
			maxMessages:    routines,
		}

		q.readPos.Store(0)
		q.writePos.Store(50)

		_, err := q.Push(generateBytes())

		if err == nil {
			t.Error("queue should not have accepted the request")
		}
	})

	t.Run("push very large message", func(t *testing.T) {

		fullBuffer := make([][]byte, routines)

		maxSize := uint32(3)

		q := Queue{
			backingSlice:   fullBuffer,
			maxMessageSize: maxSize,
			maxMessages:    routines,
		}

		q.readPos.Store(0)
		q.writePos.Store(0)

		_, err := q.Push([]byte{1, 2})

		if err != nil {
			t.Error("queue should have accepted the request")
		}

		_, err = q.Push([]byte{1, 2, 3, 4})

		if err == nil {
			t.Error("queue should not have accepted the request")
		}
	})
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
	queue := NewQueue(mx, 9*MB)

	wgPop := sync.WaitGroup{}
	dataChan := make(chan []byte)
	failedChan := make(chan struct{})

	popper := func() {
		data, err := queue.StartPop()
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

func Test_adjust(t *testing.T) {

	t.Run("test no shifting", func(t *testing.T) {
		backing := [][]byte{
			generateBytes(),
			generateBytes(),
			generateBytes(),
			generateBytes(),
			generateBytes(),
		}

		q := Queue{
			backingSlice: backing,
			maxMessages:  5,
		}

		q.readPos.Store(5)
		q.writePos.Store(5)

		q.adjust()

		compareBytes(t, backing, q.backingSlice)
	})

	t.Run("test shift", func(t *testing.T) {
		backing := [][]byte{
			{},
			{},
			generateBytes(),
			generateBytes(),
			generateBytes(),
		}

		dupe := make([][]byte, cap(backing))
		copy(dupe, backing)

		q := Queue{
			backingSlice: dupe,
			maxMessages:  5,
		}

		q.readPos.Store(5)
		q.writePos.Store(5)

		q.adjust()

		if q.readPos.Load() != 0 {
			fmt.Println(q.readPos.Load())
			t.Error("read position was not reset")
		}

		if q.writePos.Load() != 3 {
			fmt.Println(q.writePos.Load(), q.backingSlice)
			t.Error("write position was not reset")
		}

		eq1 := bytes.Equal(backing[2], q.backingSlice[0])
		eq2 := bytes.Equal(backing[3], q.backingSlice[1])
		eq3 := bytes.Equal(backing[4], q.backingSlice[2])

		if !eq1 || !eq2 || !eq3 {
			t.Error("incorrect shift")
		}
	})

	t.Run("shift empty slice", func(t *testing.T) {
		backing := [][]byte{
			{},
			{},
			{},
			{},
			{},
		}

		q := Queue{
			backingSlice: backing,
			maxMessages:  5,
		}

		q.readPos.Store(5)
		q.writePos.Store(5)

		q.adjust()

		if q.writePos.Load() != 0 {
			t.Error("incorrect shift")
		}

		if q.readPos.Load() != 0 {
			t.Error("incorrect shift")
		}

	})
}
