package billing

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"
)

// LedgerSource is the operator-plane → cloud ledger-event seam. Production
// will be a `fleet.LedgerTail` JSON-RPC subscription on the tunnel; the
// interface keeps the subscriber decoupled from the transport.
//
// Next blocks until the next bill event is available or ctx is cancelled.
// Sources are expected to deliver events in arrival order but the
// subscriber tolerates out-of-order delivery (it dedupes by EntryID).
//
// On reconnect or restart the source MUST re-emit any events whose
// EntryID has not yet been acknowledged via Ack — at-least-once delivery,
// dedupe at the outbox.
type LedgerSource interface {
	Next(ctx context.Context) (LedgerEvent, error)
	Ack(ctx context.Context, entryID string) error
}

// Subscriber pumps a LedgerSource into a Service.Record loop. One
// goroutine per Subscriber; Stop blocks until the loop exits.
type Subscriber struct {
	src    LedgerSource
	svc    *Service
	stop   chan struct{}
	done   chan struct{}
	logger *log.Logger
}

// NewSubscriber wires a source to a service.
func NewSubscriber(src LedgerSource, svc *Service, logger *log.Logger) *Subscriber {
	if logger == nil {
		logger = log.Default()
	}
	return &Subscriber{
		src:    src,
		svc:    svc,
		stop:   make(chan struct{}),
		done:   make(chan struct{}),
		logger: logger,
	}
}

// Run loops Next → Record → Ack until ctx is cancelled or Stop is called.
// Returns when the loop has fully drained.
func (s *Subscriber) Run(ctx context.Context) error {
	if s.src == nil || s.svc == nil {
		return errors.New("billing: subscriber missing source or service")
	}
	defer close(s.done)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.stop:
			return nil
		default:
		}
		ev, err := s.src.Next(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return err
			}
			// Transient source error — backoff and retry. The source owns
			// reconnect; we just yield long enough to avoid a hot loop.
			s.logger.Printf("billing: subscriber: Next error: %v", err)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-s.stop:
				return nil
			case <-time.After(1 * time.Second):
			}
			continue
		}
		if ev.Result != "bill" {
			// Non-bill events still need acking so the source advances.
			_ = s.src.Ack(ctx, ev.EntryID)
			continue
		}
		if err := s.svc.Record(ctx, ev); err != nil {
			s.logger.Printf("billing: subscriber: Record error entry=%s: %v", ev.EntryID, err)
			// Do NOT ack — the source will redeliver on reconnect/restart
			// and the outbox UNIQUE(entry_id) will dedupe the eventual
			// successful insert.
			continue
		}
		if err := s.src.Ack(ctx, ev.EntryID); err != nil {
			s.logger.Printf("billing: subscriber: Ack error entry=%s: %v", ev.EntryID, err)
		}
	}
}

// Stop signals the subscriber loop to exit. Returns after Run has returned.
func (s *Subscriber) Stop() {
	close(s.stop)
	<-s.done
}

// ChannelSource is an in-memory LedgerSource backed by a Go channel. Used
// by tests and by the dev apiserver until WS-14 / operator-plane wires
// `fleet.LedgerTail` on the tunnel. Production replaces this with the
// tunnel-backed RPC client.
type ChannelSource struct {
	Events <-chan LedgerEvent
	acked  chan string
}

// NewChannelSource returns a LedgerSource that reads from ch.
func NewChannelSource(ch <-chan LedgerEvent) *ChannelSource {
	return &ChannelSource{Events: ch, acked: make(chan string, 256)}
}

// Next blocks until an event is available or ctx is done.
func (c *ChannelSource) Next(ctx context.Context) (LedgerEvent, error) {
	select {
	case <-ctx.Done():
		return LedgerEvent{}, ctx.Err()
	case ev, ok := <-c.Events:
		if !ok {
			return LedgerEvent{}, fmt.Errorf("billing: channel source closed")
		}
		return ev, nil
	}
}

// Ack records the entry id on an internal channel so tests can assert
// acknowledgement ordering. Non-blocking.
func (c *ChannelSource) Ack(_ context.Context, entryID string) error {
	select {
	case c.acked <- entryID:
	default:
	}
	return nil
}

// Acked drains and returns all entry ids acknowledged so far. Test helper.
func (c *ChannelSource) Acked() []string {
	out := []string{}
	for {
		select {
		case id := <-c.acked:
			out = append(out, id)
		default:
			return out
		}
	}
}
