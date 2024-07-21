package queue

import (
	"fmt"
	"sync/atomic"
	"testing"
)

func updateVal(v *atomic.Value) {

	val := v.Load().(int)
	v.Store(val + 1)
}

func Test_q(t *testing.T) {

	val := atomic.Value{}
	val.Store(1)

	updateVal(&val)

	fmt.Println(val.Load())
}
