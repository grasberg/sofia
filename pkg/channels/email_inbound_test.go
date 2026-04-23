package channels

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grasberg/sofia/pkg/bus"
	"github.com/grasberg/sofia/pkg/config"
)

// fakeReceiver lets tests inject a canned Poll() response.
type fakeReceiver struct {
	emails []IncomingEmail
	once   sync.Once
	done   chan struct{}
}

func newFakeReceiver(emails ...IncomingEmail) *fakeReceiver {
	return &fakeReceiver{emails: emails, done: make(chan struct{})}
}

func (r *fakeReceiver) Connect(_ context.Context) error { return nil }

func (r *fakeReceiver) Poll(_ context.Context) ([]IncomingEmail, error) {
	var out []IncomingEmail
	r.once.Do(func() {
		out = r.emails
		close(r.done)
	})
	return out, nil
}

func (r *fakeReceiver) Close() error { return nil }

// fakeIngested stubs the idempotency store.
type fakeIngested struct {
	mu    sync.Mutex
	seen  map[string]bool
	marks int
}

func (f *fakeIngested) IsEmailIngested(id string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.seen[id], nil
}

func (f *fakeIngested) MarkEmailIngested(id, _, _, _ string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen == nil {
		f.seen = map[string]bool{}
	}
	f.seen[id] = true
	f.marks++
	return nil
}

func TestEmailChannel_Autonomous_RoutesToInboundHandler(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      true,
		Username:     "test@example.com",
		PollInterval: 1,
		Autonomous:   true,
	}

	emails := []IncomingEmail{
		{From: "alice@example.com", Subject: "help", Body: "I need help", MessageID: "m1"},
	}
	recv := newFakeReceiver(emails...)

	msgBus := bus.NewMessageBus()
	ec := NewEmailChannel(cfg, msgBus)
	ec.receiver = recv
	ec.SetIngestedStore(&fakeIngested{})

	var handled atomic.Int32
	var got IncomingEmail
	var mu sync.Mutex
	ec.SetInboundHandler(func(e IncomingEmail) {
		mu.Lock()
		got = e
		mu.Unlock()
		handled.Add(1)
	})

	if err := ec.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = ec.Stop(context.Background()) }()

	// Wait for one poll cycle to run.
	select {
	case <-recv.done:
	case <-time.After(3 * time.Second):
		t.Fatal("receiver was never polled")
	}

	// Poll runs fetchAndPublish asynchronously; give handler a chance.
	deadline := time.Now().Add(2 * time.Second)
	for handled.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}

	if handled.Load() != 1 {
		t.Fatalf("handler invoked %d times, want 1", handled.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if got.From != "alice@example.com" {
		t.Errorf("handler got From=%q", got.From)
	}
}

func TestEmailChannel_NonAutonomous_UsesBus(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      true,
		Username:     "test@example.com",
		PollInterval: 1,
		Autonomous:   false, // explicit — route via bus
	}

	emails := []IncomingEmail{
		{From: "bob@example.com", Subject: "hey", Body: "hi", MessageID: "m2"},
	}
	recv := newFakeReceiver(emails...)

	msgBus := bus.NewMessageBus()
	ec := NewEmailChannel(cfg, msgBus)
	ec.receiver = recv
	ec.SetIngestedStore(&fakeIngested{})

	handlerCalled := atomic.Bool{}
	ec.SetInboundHandler(func(_ IncomingEmail) {
		handlerCalled.Store(true)
	})

	if err := ec.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = ec.Stop(context.Background()) }()

	<-recv.done
	time.Sleep(200 * time.Millisecond)

	if handlerCalled.Load() {
		t.Error("inbound handler must not run when Autonomous=false")
	}
}

func TestEmailChannel_DedupeSkipsDuplicates(t *testing.T) {
	cfg := config.EmailConfig{
		Enabled:      true,
		Username:     "t@example.com",
		PollInterval: 1,
		Autonomous:   true,
	}

	emails := []IncomingEmail{
		{From: "c@example.com", Subject: "s", Body: "b", MessageID: "dup-1"},
		{From: "c@example.com", Subject: "s", Body: "b", MessageID: "dup-1"},
	}
	recv := newFakeReceiver(emails...)

	ing := &fakeIngested{}
	msgBus := bus.NewMessageBus()
	ec := NewEmailChannel(cfg, msgBus)
	ec.receiver = recv
	ec.SetIngestedStore(ing)

	var handled atomic.Int32
	ec.SetInboundHandler(func(_ IncomingEmail) { handled.Add(1) })

	if err := ec.Start(context.Background()); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = ec.Stop(context.Background()) }()

	<-recv.done
	deadline := time.Now().Add(1 * time.Second)
	for handled.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}

	if handled.Load() != 1 {
		t.Errorf("handler invoked %d times, want 1 (dedupe)", handled.Load())
	}
}
