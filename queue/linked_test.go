package queue

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

func createRandomByteMatrix(scale, columns int) [][]byte {

	randMatrix := make([][]byte, columns)

	for index, _ := range randMatrix {
		source := rand.NewPCG(uint64(index), 1024)
		gen := rand.New(source)
		tempSlice := gen.Perm(256)

		byteVector := make([]byte, 256)

		for idx, i := range tempSlice {
			byteVector[idx] = uint8(i)
		}

		randMatrix[index] = byteVector
	}

	retMatrix := make([][]byte, columns)

	for index, row := range randMatrix {
		vect := make([]byte, 0, 256*scale)
		for range scale {
			vect = append(vect, row...)
		}

		retMatrix[index] = vect
	}

	return retMatrix
}

func TestLinkedQueue_Push(t *testing.T) {

	//containsInt := func(intSlice []int, val int) bool {
	//	for _, data := range intSlice {
	//		if data == val {
	//			return true
	//		}
	//	}
	//
	//	return false
	//}
	//
	//byteContains := func(slices [][]byte, bt []byte, visited []int) []int {
	//
	//	for index, s := range slices {
	//		if bytes.Equal(s, bt) && containsInt(visited, index) {
	//			return []int{}
	//		}
	//
	//		if bytes.Equal(s, bt) {
	//			return append(visited, index)
	//		}
	//	}
	//
	//	return []int{}
	//}
	//
	//queueContains := func(m *message, slices [][]byte) (bool, int) {
	//
	//	var sl []int
	//	size := 0
	//	for m != nil {
	//		size += 1
	//		sl = byteContains(slices, m.buffer, sl)
	//		if len(sl) == 0 {
	//			fmt.Println("here")
	//			return false, 0
	//		}
	//		m = m.next
	//	}
	//
	//	return true, size
	//}

	bs := createRandomByteMatrix(409, 100_000)

	t.Run("sequential push", func(t *testing.T) {
		q := LinkedQueue{
			maxMessageSize: MB,
			cap:            100_000,
			maxHiddenTime:  time.Duration(10) * time.Second,
			killCh:         make(chan struct{}),
		}

		t1 := time.Now()
		for _, row := range bs {
			pos, err := q.Push(row)

			if err != nil {
				t.Fatal(err)
			}

			if pos != uint32(q.Len()) {
				t.Fatal("pos and size do not match")
			}
		}
		t2 := time.Now()
		fmt.Println(t2.Sub(t1).Milliseconds())

		//passed, size := queueContains(q.head, bs)
		//
		//if !passed {
		//	t.Fatal("uploads failed")
		//}
		//
		//if int32(size) != q.Len() {
		//	t.Fatal("traversed size does not match true size")
		//}
	})

	t.Run("push concurrently", func(t *testing.T) {
		q := LinkedQueue{
			maxMessageSize: MB,
			cap:            100_000,
			maxHiddenTime:  time.Duration(10) * time.Second,
			killCh:         make(chan struct{}),
		}

		//errChan := make(chan error)
		wg := sync.WaitGroup{}
		wg1 := sync.WaitGroup{}
		blocker := make(chan struct{})

		for _, b := range bs {

			wg.Add(1)
			wg1.Add(1)
			go func() {
				wg.Done()
				for range blocker {

				}
				defer wg1.Done()
				//_, err := q.Push(b)
				_, _ = q.Push(b)

				//if err != nil {
				//	errChan <- err
				//}
			}()
		}

		wg.Wait()
		close(blocker)
		t1 := time.Now()
		wg1.Wait()
		t2 := time.Now()

		fmt.Println(t2.Sub(t1).Milliseconds())

		//go func() {
		//	wg.Wait()
		//	close(errChan)
		//}()
		//
		//for err := range errChan {
		//	if err != nil {
		//		t.Fatal(err)
		//	}
		//}
		//t2 := time.Now()
		//fmt.Println(t2.Sub(t1).Milliseconds())

		//if q.Len() != 10_000 {
		//	t.Fatal("failed to add all 100 elements")
		//}
	})
}
