package bus

import (
	"context"
	"log"
	"sync"
	"time"
)

type MessageBus struct {
	inbound  chan InboundMessage
	outbound chan OutboundMessage
	handlers map[string]MessageHandler
	closed   bool
	mu       sync.RWMutex
}

func NewMessageBus() *MessageBus {
	return &MessageBus{
		inbound:  make(chan InboundMessage, 500),
		outbound: make(chan OutboundMessage, 500),
		handlers: make(map[string]MessageHandler),
	}
}

func (mb *MessageBus) PublishInbound(msg InboundMessage) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	if mb.closed {
		return
	}
	select {
	case mb.inbound <- msg:
	default:
		log.Printf("[bus] WARNING: inbound buffer full, dropping message from %s/%s", msg.Channel, msg.SenderID)
	}
}

func (mb *MessageBus) ConsumeInbound(ctx context.Context) (InboundMessage, bool) {
	select {
	case msg, ok := <-mb.inbound:
		return msg, ok
	case <-ctx.Done():
		return InboundMessage{}, false
	}
}

// PublishOutbound sends an outbound message. For content messages (responses),
// it blocks up to 10 seconds to avoid silently losing user-facing replies.
// Thinking indicators and other ephemeral messages use non-blocking send.
func (mb *MessageBus) PublishOutbound(msg OutboundMessage) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	if mb.closed {
		return
	}

	// Ephemeral messages (thinking indicators, stream deltas) can be dropped safely.
	if msg.Type == "thinking" || msg.Type == "stream_delta" {
		select {
		case mb.outbound <- msg:
		default:
			// Dropping ephemeral message is acceptable.
		}
		return
	}

	// Content messages must not be silently lost — block with timeout.
	select {
	case mb.outbound <- msg:
	default:
		log.Printf("[bus] WARNING: outbound buffer full, waiting to deliver response to %s/%s", msg.Channel, msg.ChatID)
		timer := time.NewTimer(10 * time.Second)
		defer timer.Stop()
		select {
		case mb.outbound <- msg:
		case <-timer.C:
			log.Printf("[bus] ERROR: outbound buffer full for 10s, dropping response to %s/%s (len=%d)",
				msg.Channel, msg.ChatID, len(msg.Content))
		}
	}
}

func (mb *MessageBus) SubscribeOutbound(ctx context.Context) (OutboundMessage, bool) {
	select {
	case msg, ok := <-mb.outbound:
		return msg, ok
	case <-ctx.Done():
		return OutboundMessage{}, false
	}
}

func (mb *MessageBus) RegisterHandler(channel string, handler MessageHandler) {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	mb.handlers[channel] = handler
}

func (mb *MessageBus) GetHandler(channel string) (MessageHandler, bool) {
	mb.mu.RLock()
	defer mb.mu.RUnlock()
	handler, ok := mb.handlers[channel]
	return handler, ok
}

func (mb *MessageBus) Close() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	if mb.closed {
		return
	}
	mb.closed = true
	close(mb.inbound)
	close(mb.outbound)
}
