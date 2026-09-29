package email

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const deliveryTimeout = 15 * time.Second

var (
	ErrDispatcherClosed = errors.New("email dispatcher is closed")
	ErrQueueFull        = errors.New("email delivery queue is full")
)

type Sender interface {
	SendOTP(context.Context, string, string) error
}

type delivery struct {
	recipient string
	code      string
}

// Dispatcher accepts email work without making the HTTP request wait for SMTP.
// A bounded queue and fixed worker count prevent unbounded goroutine growth.
type Dispatcher struct {
	sender Sender
	jobs   chan delivery
	done   chan struct{}
	cancel context.CancelFunc

	mu     sync.RWMutex
	closed bool
}

func NewDispatcher(sender Sender, workers, queueSize int) *Dispatcher {
	if workers < 1 {
		workers = 1
	}
	if queueSize < 1 {
		queueSize = 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	dispatcher := &Dispatcher{
		sender: sender,
		jobs:   make(chan delivery, queueSize),
		done:   make(chan struct{}),
		cancel: cancel,
	}
	var workersDone sync.WaitGroup
	workersDone.Add(workers)
	for range workers {
		go func() {
			defer workersDone.Done()
			dispatcher.run(ctx)
		}()
	}
	go func() {
		workersDone.Wait()
		close(dispatcher.done)
	}()
	return dispatcher
}

// SendOTP queues a copy of the delivery data and returns immediately.
func (d *Dispatcher) SendOTP(_ context.Context, recipient, code string) error {
	if d == nil || d.sender == nil {
		return ErrDispatcherClosed
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return ErrDispatcherClosed
	}
	select {
	case d.jobs <- delivery{recipient: recipient, code: code}:
		return nil
	default:
		return ErrQueueFull
	}
}

func (d *Dispatcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		select {
		case <-ctx.Done():
			return
		case job, ok := <-d.jobs:
			if !ok {
				return
			}
			deliveryCtx, cancel := context.WithTimeout(ctx, deliveryTimeout)
			err := d.sender.SendOTP(deliveryCtx, job.recipient, job.code)
			cancel()
			if err != nil {
				slog.Error("otp email delivery failed", "error", err)
			}
		}
	}
}

// Close drains queued work until ctx expires, then cancels active deliveries.
func (d *Dispatcher) Close(ctx context.Context) error {
	if d == nil {
		return nil
	}
	d.mu.Lock()
	if !d.closed {
		d.closed = true
		close(d.jobs)
	}
	d.mu.Unlock()

	select {
	case <-d.done:
		d.cancel()
		return nil
	case <-ctx.Done():
		d.cancel()
		return fmt.Errorf("closing email dispatcher: %w", ctx.Err())
	}
}
