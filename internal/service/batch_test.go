package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestBatchReportsPerIDResultsInOrder(t *testing.T) {
	ids := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	var inFlight, peak atomic.Int32

	results := Batch(context.Background(), ids, func(_ context.Context, id int) error {
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
		if id == 7 {
			return errors.New("rejected")
		}
		return nil
	})

	if len(results) != 10 {
		t.Fatalf("got %d results", len(results))
	}
	for i, r := range results {
		if r.ID != ids[i] {
			t.Errorf("result %d has id %d, want input order", i, r.ID)
		}
	}
	if failed := Failed(results); len(failed) != 1 || failed[0] != 7 {
		t.Errorf("failed = %v, want [7]", failed)
	}
	if p := peak.Load(); p > MaxInFlight {
		t.Errorf("peak concurrency %d exceeds %d", p, MaxInFlight)
	}
	if p := peak.Load(); p < 2 {
		t.Errorf("peak concurrency %d: batch should run in parallel", p)
	}
}

func TestBatchStopsStartingWorkWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var ran atomic.Int32
	results := Batch(ctx, []int{1, 2, 3}, func(context.Context, int) error {
		ran.Add(1)
		return nil
	})
	if ran.Load() != 0 {
		t.Errorf("ran %d ops after cancel", ran.Load())
	}
	for _, r := range results {
		if !errors.Is(r.Err, context.Canceled) {
			t.Errorf("result %d err = %v, want context.Canceled", r.ID, r.Err)
		}
	}
}

func TestBatchEmpty(t *testing.T) {
	if r := Batch(context.Background(), nil, func(context.Context, int) error { return nil }); len(r) != 0 {
		t.Errorf("got %v", r)
	}
}
