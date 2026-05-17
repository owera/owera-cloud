package erasure

import (
	"context"
	"time"

	"github.com/owera/owera-cloud/api/internal/queue"
)

// AdaptQueue wraps a queue.Queue so it satisfies erasure.Queue. The
// adapter is the seam where erasure stays decoupled from the concrete
// queue implementation (WS-14 owns queue.Queue; we only need a slice
// of its surface).
func AdaptQueue(q queue.Queue) Queue { return &queueAdapter{q: q} }

type queueAdapter struct {
	q queue.Queue
}

func (a *queueAdapter) Enqueue(ctx context.Context, tenantID, jobID string, payload map[string]any, idempotencyKey string) (queueItem, bool, error) {
	item, dedup, err := a.q.Enqueue(ctx, tenantID, jobID, payload, idempotencyKey)
	if err != nil {
		return nil, dedup, err
	}
	return wrapItem(item), dedup, nil
}

func (a *queueAdapter) Dequeue(ctx context.Context, claimToken string, maxAge time.Duration) (queueItem, error) {
	item, err := a.q.Dequeue(ctx, claimToken, maxAge)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, nil
	}
	return wrapItem(item), nil
}

func (a *queueAdapter) Ack(ctx context.Context, itemID, claimToken string) error {
	return a.q.Ack(ctx, itemID, claimToken)
}

func (a *queueAdapter) Nack(ctx context.Context, itemID, claimToken string) error {
	return a.q.Nack(ctx, itemID, claimToken)
}

type itemWrapper struct{ inner *queue.Item }

func wrapItem(i *queue.Item) queueItem {
	if i == nil {
		return nil
	}
	return &itemWrapper{inner: i}
}

func (w *itemWrapper) GetID() string              { return w.inner.ID }
func (w *itemWrapper) GetTenantID() string        { return w.inner.TenantID }
func (w *itemWrapper) GetJobID() string           { return w.inner.JobID }
func (w *itemWrapper) GetPayload() map[string]any { return w.inner.Payload }
