package email

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type recordingSender struct {
	started chan struct{}
	release chan struct{}

	mu         sync.Mutex
	deliveries []delivery
}

type senderFunc func(context.Context, string, string) error

func (send senderFunc) SendOTP(ctx context.Context, recipient, code string) error {
	return send(ctx, recipient, code)
}

func (s *recordingSender) SendOTP(ctx context.Context, recipient, code string) error {
	if s.started != nil {
		select {
		case s.started <- struct{}{}:
		default:
		}
	}
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	s.mu.Lock()
	s.deliveries = append(s.deliveries, delivery{recipient: recipient, code: code})
	s.mu.Unlock()
	return nil
}

func TestDispatcherQueuesWithoutWaitingForDelivery(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{started: make(chan struct{}, 1), release: make(chan struct{})}
	dispatcher := NewDispatcher(sender, 1, 1)
	requestCtx, cancelRequest := context.WithCancel(t.Context())
	if err := dispatcher.SendOTP(requestCtx, "ali@example.com", "123456"); err != nil {
		t.Fatal(err)
	}
	cancelRequest()
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("queued delivery did not start")
	}
	close(sender.release)
	closeCtx, cancelClose := context.WithTimeout(context.Background(), time.Second)
	defer cancelClose()
	if err := dispatcher.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if len(sender.deliveries) != 1 {
		t.Fatalf("deliveries = %d, want 1", len(sender.deliveries))
	}
}

func TestDispatcherRejectsWorkWhenQueueIsFullOrClosed(t *testing.T) {
	t.Parallel()
	sender := &recordingSender{started: make(chan struct{}, 1), release: make(chan struct{})}
	dispatcher := NewDispatcher(sender, 1, 1)
	if err := dispatcher.SendOTP(t.Context(), "first@example.com", "111111"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("first delivery did not start")
	}
	if err := dispatcher.SendOTP(t.Context(), "second@example.com", "222222"); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.SendOTP(t.Context(), "third@example.com", "333333"); !errors.Is(err, ErrQueueFull) {
		t.Fatalf("SendOTP() error = %v, want ErrQueueFull", err)
	}
	close(sender.release)
	closeCtx, cancelClose := context.WithTimeout(context.Background(), time.Second)
	defer cancelClose()
	if err := dispatcher.Close(closeCtx); err != nil {
		t.Fatal(err)
	}
	if err := dispatcher.SendOTP(t.Context(), "after@example.com", "444444"); !errors.Is(err, ErrDispatcherClosed) {
		t.Fatalf("SendOTP() after Close = %v, want ErrDispatcherClosed", err)
	}
}

func TestDispatcherCloseHonorsDeadline(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	release := make(chan struct{})
	dispatcher := NewDispatcher(senderFunc(func(context.Context, string, string) error {
		close(started)
		<-release
		return nil
	}), 1, 1)
	if err := dispatcher.SendOTP(t.Context(), "ali@example.com", "123456"); err != nil {
		t.Fatal(err)
	}
	<-started
	closeCtx, cancelClose := context.WithCancel(context.Background())
	cancelClose()
	if err := dispatcher.Close(closeCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Close() error = %v, want context.Canceled", err)
	}
	close(release)
	if err := dispatcher.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
}
