package bus

type InboundMessage struct {
	Channel    string            `json:"channel"`
	SenderID   string            `json:"sender_id"`
	ChatID     string            `json:"chat_id"`
	Content    string            `json:"content"`
	Media      []string          `json:"media,omitempty"`
	SessionKey string            `json:"session_key"`
	Metadata   map[string]string `json:"metadata,omitempty"`
}

type OutboundMessage struct {
	Channel  string `json:"channel"`
	ChatID   string `json:"chat_id"`
	Content  string `json:"content"`
	Type     string `json:"type,omitempty"`      // e.g., "thinking", "stream_start", "stream_delta", "stream_end"
	StreamID string `json:"stream_id,omitempty"` // Identifies a streaming session
}

type MessageHandler func(InboundMessage) error
