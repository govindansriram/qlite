package queue

import (
	"sync"
	"testing"
)

const concurrent = 100_000

func BenchmarkBuffChan(b *testing.B) {
	wg := sync.WaitGroup{}
	dataArr := generateData(concurrent)
	qs := make([]*BuffChan, 0, b.N)

	for i := 0; i < b.N; i++ {
		q := NewBuffChan()

		b.Run("test concurrent writes", func(b *testing.B) {
			for _, data := range dataArr {
				wg.Add(1)
				go func() {
					defer wg.Done()
					q.Enqueue(data)
				}()
			}

			wg.Wait()
		})

		qs = append(qs, q)
	}

	for i := 0; i < b.N; i++ {
		q := qs[i]
		b.Run("test concurrent reads", func(b *testing.B) {
			for range dataArr {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = q.Dequeue()
				}()
			}

			wg.Wait()
		})
	}
}

func BenchmarkNewBuffChan_RW(b *testing.B) {
	dataArr := generateData(concurrent)

	b.Run("test concurrent writes", func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			q := NewBuffChan()
			stopChan1 := make(chan struct{})
			stopChan2 := make(chan struct{})
			go func() {
				wg := sync.WaitGroup{}
				for _, data := range dataArr {
					wg.Add(1)
					go func() {
						defer wg.Done()
						q.Enqueue(data)
					}()
				}

				wg.Wait()
				stopChan1 <- struct{}{}
			}()

			go func() {
				wg := sync.WaitGroup{}
				for range dataArr {
					wg.Add(1)
					go func() {
						defer wg.Done()
						q.Dequeue()
					}()

					wg.Wait()
				}

				stopChan2 <- struct{}{}
			}()

			<-stopChan1
			<-stopChan2
		}

	})
}

func BenchmarkNewLinkedQueue_RW(b *testing.B) {
	dataArr := generateData(concurrent)

	b.Run("test concurrent writes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			q := LinkedQueue{}
			stopChan1 := make(chan struct{})
			stopChan2 := make(chan struct{})
			go func() {
				wg := sync.WaitGroup{}
				for _, data := range dataArr {
					wg.Add(1)
					go func() {
						defer wg.Done()
						q.Enqueue(data)
					}()
				}

				wg.Wait()
				stopChan1 <- struct{}{}
			}()

			go func() {
				wg := sync.WaitGroup{}
				for range dataArr {
					wg.Add(1)
					go func() {
						defer wg.Done()
						q.Dequeue()
					}()

					wg.Wait()
				}

				stopChan2 <- struct{}{}
			}()

			<-stopChan1
			<-stopChan2
		}
	})
}

func BenchmarkNewMS_RW(b *testing.B) {
	dataArr := generateData(concurrent)

	b.Run("test concurrent writes", func(b *testing.B) {

		for i := 0; i < b.N; i++ {
			q := NewMichaelScott()
			stopChan1 := make(chan struct{})
			stopChan2 := make(chan struct{})

			go func() {
				wg := sync.WaitGroup{}
				for _, data := range dataArr {
					wg.Add(1)
					go func() {
						defer wg.Done()
						q.Enqueue(data)
					}()
				}

				wg.Wait()
				stopChan1 <- struct{}{}
			}()

			go func() {
				wg := sync.WaitGroup{}
				for range dataArr {
					wg.Add(1)
					go func() {
						defer wg.Done()
						q.Dequeue()
					}()

					wg.Wait()
				}

				stopChan2 <- struct{}{}
			}()

			<-stopChan1
			<-stopChan2
		}
	})
}

func BenchmarkMichaelScott(b *testing.B) {
	wg := sync.WaitGroup{}
	dataArr := generateData(concurrent)
	qs := make([]*MichaelScott, 0, b.N)

	for i := 0; i < b.N; i++ {
		q := NewMichaelScott()

		b.Run("test concurrent writes", func(b *testing.B) {
			for _, data := range dataArr {
				wg.Add(1)
				go func() {
					defer wg.Done()
					q.Enqueue(data)
				}()
			}

			wg.Wait()
		})

		qs = append(qs, q)
	}

	for i := 0; i < b.N; i++ {
		q := qs[i]
		b.Run("test concurrent reads", func(b *testing.B) {
			for range dataArr {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = q.Dequeue()
				}()
			}

			wg.Wait()
		})
	}
}

func BenchmarkLinkedQueue(b *testing.B) {
	wg := sync.WaitGroup{}
	dataArr := generateData(concurrent)
	qs := make([]*LinkedQueue, b.N)

	for i := 0; i < b.N; i++ {
		q := &LinkedQueue{}
		qs[i] = q

		b.Run("test concurrent writes", func(b *testing.B) {
			for _, data := range dataArr {
				wg.Add(1)
				go func() {
					defer wg.Done()
					q.Enqueue(data)
				}()
			}
			wg.Wait()
		})
	}

	for i := 0; i < b.N; i++ {
		q := qs[i]

		b.Run("test concurrent reads", func(b *testing.B) {
			for range dataArr {
				wg.Add(1)
				go func() {
					defer wg.Done()
					_, _ = q.Dequeue()
				}()
			}
			wg.Wait()
		})
	}
}
