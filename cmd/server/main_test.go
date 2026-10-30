package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRetryWaitsForDependency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	attempts := 0
	err := retry(ctx, "test", func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("attempt missing deadline")
		}
		attempts++
		if attempts == 1 {
			return errors.New("not ready")
		}
		return nil
	})
	if err != nil || attempts != 2 {
		t.Fatalf("attempts=%d err=%v", attempts, err)
	}
}

func TestRetryCancellationDoesNotLeakCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	err := retry(ctx, "database", func(context.Context) error { cancel(); return errors.New("secret password") })
	if !errors.Is(err, context.Canceled) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("unexpected error: %v", err)
	}
	called := false
	err = retry(ctx, "database", func(context.Context) error { called = true; return nil })
	if called || !errors.Is(err, context.Canceled) {
		t.Fatal("must not connect after cancellation")
	}
}
