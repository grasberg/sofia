package voice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/grasberg/sofia/pkg/logger"
)

// ElevenLabsTTS implements TTSProvider using the ElevenLabs API.
type ElevenLabsTTS struct {
	apiKey     string
	apiBase    string
	httpClient *http.Client
}

type elevenLabsRequest struct {
	Text    string `json:"text"`
	ModelID string `json:"model_id"`
}

// NewElevenLabsTTS creates a new ElevenLabs TTS provider.
func NewElevenLabsTTS(apiKey string) *ElevenLabsTTS {
	logger.DebugCF("voice", "Creating ElevenLabs TTS provider", map[string]any{
		"has_api_key": apiKey != "",
	})

	return &ElevenLabsTTS{
		apiKey:  apiKey,
		apiBase: "https://api.elevenlabs.io/v1",
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Synthesize converts text to speech using the ElevenLabs API.
// The voice parameter is the ElevenLabs voice ID.
// Returns MP3 audio bytes with content type "audio/mpeg".
func (t *ElevenLabsTTS) Synthesize(
	ctx context.Context, text string, voice string,
) ([]byte, string, error) {
	logger.InfoCF("voice", "Starting TTS synthesis", map[string]any{
		"text_length": len(text),
		"voice":       voice,
	})

	reqBody := elevenLabsRequest{
		Text:    text,
		ModelID: "eleven_multilingual_v2",
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		logger.ErrorCF("voice", "Failed to marshal request body", map[string]any{"error": err})
		return nil, "", fmt.Errorf("failed to marshal request body: %w", err)
	}

	url := fmt.Sprintf("%s/text-to-speech/%s", t.apiBase, voice)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		logger.ErrorCF("voice", "Failed to create request", map[string]any{"error": err})
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Xi-Api-Key", t.apiKey)

	logger.DebugCF("voice", "Sending TTS request to ElevenLabs API", map[string]any{
		"url":                url,
		"request_size_bytes": len(bodyBytes),
	})

	resp, err := t.httpClient.Do(req)
	if err != nil {
		logger.ErrorCF("voice", "Failed to send request", map[string]any{"error": err})
		return nil, "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.ErrorCF("voice", "Failed to read response", map[string]any{"error": err})
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		logger.ErrorCF("voice", "API error", map[string]any{
			"status_code": resp.StatusCode,
			"response":    string(body),
		})
		return nil, "", fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(body))
	}

	logger.InfoCF("voice", "TTS synthesis completed successfully", map[string]any{
		"audio_size_bytes": len(body),
		"voice":            voice,
	})

	return body, "audio/mpeg", nil
}

// IsAvailable returns true if the ElevenLabs API key is configured.
func (t *ElevenLabsTTS) IsAvailable() bool {
	available := t.apiKey != ""
	logger.DebugCF("voice", "Checking ElevenLabs TTS availability", map[string]any{
		"available": available,
	})
	return available
}
