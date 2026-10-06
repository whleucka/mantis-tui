package service

import (
	"context"
	"sync"
)

// MaxInFlight caps concurrent requests in a batch.
const MaxInFlight = 4

// Result is the outcome of one operation in a batch.
type Result struct {
	ID  int
	Err error
}

// Batch runs op for every id with at most MaxInFlight running at once and
// returns one Result per id, in input order. Ids not yet started when ctx is
// cancelled report ctx.Err() without running.
func Batch(ctx context.Context, ids []int, op func(ctx context.Context, id int) error) []Result {
	results := make([]Result, len(ids))
	sem := make(chan struct{}, MaxInFlight)
	var wg sync.WaitGroup
	for i, id := range ids {
		results[i].ID = id
		select {
		case <-ctx.Done():
			results[i].Err = ctx.Err()
			continue
		case sem <- struct{}{}:
		}
		if err := ctx.Err(); err != nil {
			<-sem
			results[i].Err = err
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			results[i].Err = op(ctx, id)
		}()
	}
	wg.Wait()
	return results
}

// Failed returns the ids whose operation failed.
func Failed(results []Result) []int {
	var ids []int
	for _, r := range results {
		if r.Err != nil {
			ids = append(ids, r.ID)
		}
	}
	return ids
}
