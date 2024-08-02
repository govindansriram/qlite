package queue

import (
	"log"
	"math/rand/v2"
	"sync"
	"testing"
)

func createRandomData(size uint32) []byte {
	length := rand.IntN(int(size)) + 1
	message := make([]byte, length)

	for pos := range message {
		message[pos] = 'a'
	}

	return message
}

func createFixedData(size uint32) []byte {
	message := make([]byte, size)

	for pos := range message {
		message[pos] = 'a'
	}

	return message
}

func concurrentPush(q *Queue, messages [][]byte) {

	wg := sync.WaitGroup{}

	for _, mess := range messages {
		wg.Add(1)

		go func() {
			_, err := q.Push(mess)
			if err != nil {
				log.Print(err)
			}
			defer wg.Done()
		}()
	}

	wg.Wait()
}

func BenchmarkQueue_Push(b *testing.B) {

	b.Run("random message size", func(b *testing.B) {
		const messageCount = 1000
		const size = MB * 8

		messages := make([][]byte, messageCount)

		for i := range messageCount {
			messages[i] = createRandomData(size)
		}

		for i := 0; i < b.N; i++ {
			q := NewQueue(messageCount, 8*MB)
			concurrentPush(&q, messages)
		}
	})

	b.Run("random message size", func(b *testing.B) {
		const messageCount = 1000
		const size = MB * 3

		messages := make([][]byte, messageCount)

		for i := range messageCount {
			messages[i] = createFixedData(size)
		}

		for i := 0; i < b.N; i++ {
			q := NewQueue(messageCount, 8*MB)
			concurrentPush(&q, messages)
		}
	})
}
