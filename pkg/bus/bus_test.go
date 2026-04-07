package bus

import (
	"context"
	"testing"
	"time"
)

func TestNewMessageBus(t *testing.T) {
	mb := NewMessageBus()
	if mb == nil {
		t.Fatal("NewMessageBus returned nil")
	}
	if mb.inbound == nil {
		t.Error("inbound channel not initialized")
	}
	if mb.outbound == nil {
		t.Error("outbound channel not initialized")
	}
	if len(mb.handlers) != 0 {
		t.Errorf("handlers map should be empty, got %d", len(mb.handlers))
	}
}

func TestPublishInbound(t *testing.T) {
	mb := NewMessageBus()
	msg := InboundMessage{
		Channel:  "telegram",
		SenderID: "user123",
		ChatID:   "chat456",
		Content:  "Hello",
	}

	mb.PublishInbound(msg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	received, ok := mb.ConsumeInbound(ctx)
	if !ok {
		t.Fatal("ConsumeInbound returned false")
	}
	if received.Content != msg.Content {
		t.Errorf("content = %q, want %q", received.Content, msg.Content)
	}
}

func TestConsumeInbound_ContextCancellation(t *testing.T) {
	mb := NewMessageBus()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok := mb.ConsumeInbound(ctx)
	if ok {
		t.Error("ConsumeInbound should return false when context is canceled")
	}
}

func TestPublishOutbound_ContentMessage(t *testing.T) {
	mb := NewMessageBus()
	msg := OutboundMessage{
		Channel: "telegram",
		ChatID:  "chat123",
		Content: "Response",
		Type:    "response",
	}

	mb.PublishOutbound(msg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	received, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("SubscribeOutbound returned false")
	}
	if received.Content != msg.Content {
		t.Errorf("content = %q, want %q", received.Content, msg.Content)
	}
}

func TestPublishOutbound_EphemeralMessage(t *testing.T) {
	mb := NewMessageBus()
	msg := OutboundMessage{
		Channel: "telegram",
		ChatID:  "chat123",
		Content: "thinking...",
		Type:    "thinking",
	}

	mb.PublishOutbound(msg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	received, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("SubscribeOutbound returned false")
	}
	if received.Type != "thinking" {
		t.Errorf("type = %q, want %q", received.Type, "thinking")
	}
}

func TestSubscribeOutbound_ContextCancellation(t *testing.T) {
	mb := NewMessageBus()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, ok := mb.SubscribeOutbound(ctx)
	if ok {
		t.Error("SubscribeOutbound should return false when context is canceled")
	}
}

func TestRegisterHandler(t *testing.T) {
	mb := NewMessageBus()
	handler := func(msg InboundMessage) error {
		return nil
	}

	mb.RegisterHandler("telegram", handler)

	retrievedHandler, ok := mb.GetHandler("telegram")
	if !ok {
		t.Fatal("GetHandler should return true for registered handler")
	}
	if retrievedHandler == nil {
		t.Error("retrieved handler is nil")
	}
}

func TestGetHandler_NotFound(t *testing.T) {
	mb := NewMessageBus()
	_, ok := mb.GetHandler("nonexistent")
	if ok {
		t.Error("GetHandler should return false for unregistered handler")
	}
}

func TestClose(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()

	if !mb.closed {
		t.Error("MessageBus should be marked as closed")
	}
}

func TestPublishInbound_AfterClose(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()

	msg := InboundMessage{
		Channel:  "telegram",
		SenderID: "user123",
		ChatID:   "chat456",
		Content:  "Hello",
	}

	// Should not panic
	mb.PublishInbound(msg)
}

func TestPublishOutbound_AfterClose(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()

	msg := OutboundMessage{
		Channel: "telegram",
		ChatID:  "chat123",
		Content: "Response",
	}

	// Should not panic
	mb.PublishOutbound(msg)
}

func TestClose_Idempotent(t *testing.T) {
	mb := NewMessageBus()
	mb.Close()
	mb.Close() // Second close should not panic
}

func TestMultipleHandlers(t *testing.T) {
	mb := NewMessageBus()

	handler1 := func(msg InboundMessage) error { return nil }
	handler2 := func(msg InboundMessage) error { return nil }

	mb.RegisterHandler("telegram", handler1)
	mb.RegisterHandler("discord", handler2)

	h1, ok1 := mb.GetHandler("telegram")
	h2, ok2 := mb.GetHandler("discord")

	if !ok1 || !ok2 {
		t.Fatal("both handlers should be registered")
	}
	if h1 == nil || h2 == nil {
		t.Error("handlers should not be nil")
	}
}

func TestConcurrentPublishInbound(t *testing.T) {
	mb := NewMessageBus()

	done := make(chan bool, 2)

	go func() {
		for i := 0; i < 10; i++ {
			mb.PublishInbound(InboundMessage{
				Channel:  "telegram",
				SenderID: "user1",
				ChatID:   "chat1",
				Content:  "msg1",
			})
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 10; i++ {
			mb.PublishInbound(InboundMessage{
				Channel:  "discord",
				SenderID: "user2",
				ChatID:   "chat2",
				Content:  "msg2",
			})
		}
		done <- true
	}()

	<-done
	<-done

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	count := 0
	for count < 20 {
		_, ok := mb.ConsumeInbound(ctx)
		if !ok {
			break
		}
		count++
	}

	if count != 20 {
		t.Errorf("expected 20 messages, got %d", count)
	}
}

func TestStreamDeltaMessage(t *testing.T) {
	mb := NewMessageBus()
	msg := OutboundMessage{
		Channel:  "telegram",
		ChatID:   "chat123",
		Content:  "partial",
		Type:     "stream_delta",
		StreamID: "stream1",
	}

	mb.PublishOutbound(msg)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	received, ok := mb.SubscribeOutbound(ctx)
	if !ok {
		t.Fatal("SubscribeOutbound returned false")
	}
	if received.StreamID != "stream1" {
		t.Errorf("stream_id = %q, want %q", received.StreamID, "stream1")
	}
}
