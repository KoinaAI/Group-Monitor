package main

import (
	"context"
	"sync"
)

const maxNotifyWorkers = 8

type notifyResult struct {
	userID int64
	err    error
}

// sendPrivateConcurrent fans out independent DMs with a bounded worker pool so
// one slow or unavailable recipient cannot serialize every other notification.
func sendPrivateConcurrent(ob *OneBot, masters []Master, text string) (int, []int64) {
	return sendPrivateConcurrentContext(context.Background(), ob, masters, text)
}

func sendPrivateConcurrentContext(ctx context.Context, ob *OneBot, masters []Master, text string) (int, []int64) {
	if len(masters) == 0 {
		return 0, nil
	}
	workers := len(masters)
	if workers > maxNotifyWorkers {
		workers = maxNotifyWorkers
	}
	jobs := make(chan Master)
	results := make(chan notifyResult, len(masters))
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for m := range jobs {
				if err := ctx.Err(); err != nil {
					results <- notifyResult{userID: m.UserID, err: err}
					continue
				}
				results <- notifyResult{userID: m.UserID, err: ob.SendPrivateMsgContext(ctx, m.UserID, text)}
			}
		}()
	}
	go func() {
		for _, m := range masters {
			if ctx.Err() != nil {
				break
			}
			jobs <- m
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()

	sent := 0
	failed := make([]int64, 0)
	for result := range results {
		if result.err != nil {
			failed = append(failed, result.userID)
			continue
		}
		sent++
	}
	return sent, failed
}
