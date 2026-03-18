package voice

import "context"

// TTSProvider synthesizes speech from text.
type TTSProvider interface {
	Synthesize(ctx context.Context, text string, voice string) (audio []byte, contentType string, err error)
	IsAvailable() bool
}

// NewTTSProvider returns the appropriate TTS provider based on the given provider name
// and API key. Supported providers: "elevenlabs", "system".
// Falls back to SystemTTS if the provider name is unknown or empty.
func NewTTSProvider(provider string, apiKey string) TTSProvider {
	switch provider {
	case "elevenlabs":
		return NewElevenLabsTTS(apiKey)
	case "system":
		return NewSystemTTS()
	default:
		return NewSystemTTS()
	}
}
