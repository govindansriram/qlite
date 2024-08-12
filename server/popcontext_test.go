package server

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPopContext_Deadline(t *testing.T) {

	t.Run("context cancelled", func(t *testing.T) {
		operatingTime := time.Second * 4
		ctx, cf := NewPopContext(nil, operatingTime)

		go cf()

		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.Canceled) {
				t.Error("wrong cancellation reason")
			}
		case <-ctx.Removed():
			t.Error("context was inappropriately removed")
		}
	})

	t.Run("deadline exceeded", func(t *testing.T) {
		operatingTime := time.Second * 0
		ctx, cf := NewPopContext(nil, operatingTime)

		defer cf()

		select {
		case <-ctx.Done():
			if !errors.Is(ctx.Err(), context.DeadlineExceeded) {
				t.Error("wrong cancellation reason")
			}
		case <-ctx.Removed():
			t.Error("context was inappropriately removed")
		}
	})

	t.Run("context removed", func(t *testing.T) {
		operatingTime := time.Second * 0
		ctx, cf := NewPopContext(nil, operatingTime)

		defer cf()

		errChan := make(chan error)

		go func() {
			err := ctx.Remove()
			if err != nil {
				errChan <- err
			}
		}()

		select {
		case <-ctx.Done():
			t.Error("context was cancelled")
		case <-ctx.Removed():
			return
		case err := <-errChan:
			t.Fatal(err)
		}
	})
}
