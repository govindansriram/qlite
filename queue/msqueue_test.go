package queue

import (
	"encoding/binary"
	"sync"
	"sync/atomic"
	"testing"
)

const STOP = 100

func generateData(stop uint32) [][]byte {
	dataArr := make([][]byte, 0, stop)
	for i := range stop {
		hold := make([]byte, 4)
		binary.LittleEndian.PutUint32(hold, i)
		dataArr = append(dataArr, hold)
	}

	return dataArr
}

func sum(stop uint32) uint32 {
	var total uint32
	for i := range stop {
		total += i
	}

	return total
}

func TestMichaelScott_Enqueue(t *testing.T) {

	t.Run("michael scott sequential", func(t *testing.T) {
		msq := NewMichaelScott()

		dataArr := generateData(STOP)

		for idx, data := range dataArr {
			pos := msq.Enqueue(data)

			if int32(idx)+1 != pos {
				t.Error("position and index do not match")
			}
		}

		if msq.Len() != STOP {
			t.Fatal("length in incorrect")
		}

		total := sum(STOP)

		nxt := msq.head.Load().next.Load()
		for nxt != nil {
			data := binary.LittleEndian.Uint32(nxt.value)
			total -= data
			nxt = nxt.next.Load()
		}

		if total != 0 {
			t.Fatal("all values were not stored")
		}
	})

	t.Run("michael scott concurrent", func(t *testing.T) {
		msq := NewMichaelScott()
		dataArr := generateData(STOP)
		wg := sync.WaitGroup{}

		for _, data := range dataArr {
			wg.Add(1)
			go func() {
				defer wg.Done()
				msq.Enqueue(data)

			}()
		}

		wg.Wait()

		if msq.Len() != STOP {
			t.Fatal("length in incorrect")
		}

		total := sum(STOP)

		nxt := msq.head.Load().next.Load()
		for nxt != nil {
			data := binary.LittleEndian.Uint32(nxt.value)
			total -= data
			nxt = nxt.next.Load()
		}

		if total != 0 {
			t.Fatal("all values were not stored")
		}
	})
}

func TestMichaelScott_Dequeue(t *testing.T) {
	t.Run("test sequential dequeue", func(t *testing.T) {
		msq := NewMichaelScott()
		dataArr := generateData(STOP)

		for idx, data := range dataArr {
			pos := msq.Enqueue(data)

			if int32(idx)+1 != pos {
				t.Error("position and index do not match")
			}
		}

		length := msq.Len()
		total := sum(STOP)

		for range dataArr {
			data, err := msq.Dequeue()
			if err != nil {
				t.Error(err)
			}

			if msq.Len() != length-1 {
				t.Error("size is not properly decremented")
			}

			length = msq.Len()

			total -= binary.LittleEndian.Uint32(data)
		}

		if total != 0 {
			t.Fatal("failed to pop all data")
		}

		msq.len.Store(1)

		_, err := msq.Dequeue()

		if err == nil {
			t.Fatal("popped non existent data")
		}
	})

	t.Run("test concurrent dequeue", func(t *testing.T) {
		msq := NewMichaelScott()
		dataArr := generateData(STOP)

		for idx, data := range dataArr {
			pos := msq.Enqueue(data)

			if int32(idx)+1 != pos {
				t.Error("position and index do not match")
			}
		}

		total := atomic.Int32{}
		total.Store(int32(sum(STOP)))

		wg := sync.WaitGroup{}

		for range dataArr {
			wg.Add(1)

			go func() {
				defer wg.Done()
				data, err := msq.Dequeue()
				total.Add(-1 * int32(binary.LittleEndian.Uint32(data)))

				if err != nil {
					t.Error(err)
				}
			}()
		}

		wg.Wait()

		if msq.Len() != 0 {
			t.Fatal("failed to decrement properly")
		}

		if total.Load() != 0 {
			t.Fatal("failed to pop all data")
		}

		msq.len.Store(1)

		_, err := msq.Dequeue()

		if err == nil {
			t.Fatal("popped non existent data")
		}
	})
}

func TestMichaelScott_RW(t *testing.T) {
	msq := NewMichaelScott()
	dataArr := generateData(STOP)

	wg := sync.WaitGroup{}
	total := atomic.Int32{}
	total.Store(int32(sum(STOP)))

	wg2 := sync.WaitGroup{}
	done1 := make(chan struct{})
	done2 := make(chan struct{})

	go func() {
		for _, data := range dataArr {
			wg.Add(1)
			go func() {
				defer wg.Done()
				msq.Enqueue(data)
			}()
		}
		done1 <- struct{}{}
	}()

	go func() {
		for range dataArr {
			wg2.Add(1)
			go func() {
				defer wg2.Done()
				dt, err := msq.Dequeue()
				if err == nil {
					total.Add(int32(binary.LittleEndian.Uint32(dt)) * -1)
				}
			}()
		}
		done2 <- struct{}{}
	}()

	<-done2
	wg2.Wait()
	<-done1
	wg.Wait()

	var err error
	var dt []byte

	for err == nil {
		dt, err = msq.Dequeue()
		if err == nil {
			total.Add(int32(binary.LittleEndian.Uint32(dt)) * -1)
		}
	}

	if total.Load() != 0 {
		t.Fatal("failed to dequeue and enqueue at the same time")
	}

	_, err = msq.Dequeue()

	if err == nil {
		t.Fatal("dequeued empty queue")
	}
}
