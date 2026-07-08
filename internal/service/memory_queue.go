package service

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"
)

const (
	memoryBatchSize         = 5
	memoryBatchMaxIdle      = 2 * time.Minute
	memoryBatchTickerPeriod = 10 * time.Second
)

type pendingMemoryQueue struct {
	mu       sync.Mutex
	maxItems int
	maxIdle  time.Duration
	buckets  map[string][]memorySource
}

type pendingMemoryBatch struct {
	Key    string
	Items  []memorySource
	Reason string
}

func newPendingMemoryQueue(maxItems int, maxIdle time.Duration) *pendingMemoryQueue {
	return &pendingMemoryQueue{
		maxItems: maxItems,
		maxIdle:  maxIdle,
		buckets:  map[string][]memorySource{},
	}
}

func (q *pendingMemoryQueue) Enqueue(source memorySource) *pendingMemoryBatch {
	if q == nil {
		return nil
	}
	key := memoryBucketKey(source.GroupID)

	q.mu.Lock()
	defer q.mu.Unlock()

	source.QueuedAt = time.Now()
	q.buckets[key] = append(q.buckets[key], source)
	size := len(q.buckets[key])
	log.Printf("【待记忆入队】bucket=%s size=%d group=%s user=%s message=%s",
		key, size, source.GroupID, source.UserID, source.MessageID)

	if size < q.maxItems {
		return nil
	}
	items := append([]memorySource(nil), q.buckets[key]...)
	delete(q.buckets, key)
	return &pendingMemoryBatch{
		Key:    key,
		Items:  items,
		Reason: "count",
	}
}

func (q *pendingMemoryQueue) FlushExpired(now time.Time) []pendingMemoryBatch {
	if q == nil {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	batches := make([]pendingMemoryBatch, 0)
	for key, items := range q.buckets {
		if len(items) == 0 {
			delete(q.buckets, key)
			continue
		}
		last := items[len(items)-1]
		if now.Sub(last.QueuedAt) < q.maxIdle {
			continue
		}
		copied := append([]memorySource(nil), items...)
		delete(q.buckets, key)
		batches = append(batches, pendingMemoryBatch{
			Key:    key,
			Items:  copied,
			Reason: "timeout",
		})
	}
	return batches
}

func (s *Service) runMemoryBatchLoop() {
	ticker := time.NewTicker(memoryBatchTickerPeriod)
	defer ticker.Stop()

	for range ticker.C {
		if s == nil || s.memoryPending == nil {
			return
		}
		batches := s.memoryPending.FlushExpired(time.Now())
		for _, batch := range batches {
			log.Printf("【待记忆批处理触发】reason=%s bucket=%s size=%d", batch.Reason, batch.Key, len(batch.Items))
			s.processMemoryBatch(context.Background(), batch)
		}
	}
}

func (s *Service) enqueueMemorySource(source *memorySource) {
	if s == nil || s.memoryPending == nil || source == nil {
		log.Printf("【待记忆入队跳过】reason=service_or_queue_nil")
		return
	}
	source.Content = strings.TrimSpace(source.Content)
	if source.Content == "" {
		log.Printf("【待记忆入队跳过】group=%s user=%s message=%s reason=empty_content",
			source.GroupID, source.UserID, source.MessageID)
		return
	}

	if batch := s.memoryPending.Enqueue(*source); batch != nil {
		log.Printf("【待记忆批处理触发】reason=%s bucket=%s size=%d", batch.Reason, batch.Key, len(batch.Items))
		s.processMemoryBatch(context.Background(), *batch)
	}
}

func memoryBucketKey(groupID string) string {
	return groupID
}
