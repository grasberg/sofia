package voice

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestElevenLabsTTS_Synthesize(t *testing.T) {
	fakeAudio := []byte("fake-mp3-audio-data")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		assert.Equal(t, "POST", r.Method)
		assert.Contains(t, r.URL.Path, "/v1/text-to-speech/test-voice-id")

		// Verify headers
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "test-api-key", r.Header.Get("Xi-Api-Key"))

		// Verify request body
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		var reqBody map[string]string
		err = json.Unmarshal(body, &reqBody)
		require.NoError(t, err)
		assert.Equal(t, "Hello, world!", reqBody["text"])
		assert.Equal(t, "eleven_multilingual_v2", reqBody["model_id"])

		w.WriteHeader(http.StatusOK)
		w.Write(fakeAudio)
	}))
	defer server.Close()

	tts := NewElevenLabsTTS("test-api-key")
	tts.apiBase = server.URL + "/v1"

	audio, contentType, err := tts.Synthesize(context.Background(), "Hello, world!", "test-voice-id")
	require.NoError(t, err)
	assert.Equal(t, fakeAudio, audio)
	assert.Equal(t, "audio/mpeg", contentType)
}

func TestElevenLabsTTS_Synthesize_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": "invalid api key"}`))
	}))
	defer server.Close()

	tts := NewElevenLabsTTS("bad-key")
	tts.apiBase = server.URL + "/v1"

	audio, contentType, err := tts.Synthesize(context.Background(), "Hello", "voice-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "API error (status 401)")
	assert.Nil(t, audio)
	assert.Empty(t, contentType)
}

func TestElevenLabsTTS_IsAvailable(t *testing.T) {
	tests := []struct {
		name     string
		apiKey   string
		expected bool
	}{
		{name: "with API key", apiKey: "valid-key", expected: true},
		{name: "empty API key", apiKey: "", expected: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tts := NewElevenLabsTTS(tt.apiKey)
			assert.Equal(t, tt.expected, tts.IsAvailable())
		})
	}
}

func TestElevenLabsTTS_Constructor(t *testing.T) {
	tts := NewElevenLabsTTS("my-key")

	assert.NotNil(t, tts)
	assert.Equal(t, "my-key", tts.apiKey)
	assert.Equal(t, "https://api.elevenlabs.io/v1", tts.apiBase)
	assert.NotNil(t, tts.httpClient)
	assert.Equal(t, 30*time.Second, tts.httpClient.Timeout)
}

func TestSystemTTS_IsAvailable(t *testing.T) {
	tts := NewSystemTTS()
	// Just call it -- result depends on the system.
	_ = tts.IsAvailable()
}

func TestNewTTSProvider_ElevenLabs(t *testing.T) {
	provider := NewTTSProvider("elevenlabs", "test-key")
	assert.NotNil(t, provider)

	_, ok := provider.(*ElevenLabsTTS)
	assert.True(t, ok, "expected *ElevenLabsTTS")
}

func TestNewTTSProvider_System(t *testing.T) {
	provider := NewTTSProvider("system", "")
	assert.NotNil(t, provider)

	_, ok := provider.(*SystemTTS)
	assert.True(t, ok, "expected *SystemTTS")
}

func TestNewTTSProvider_Default(t *testing.T) {
	provider := NewTTSProvider("", "")
	assert.NotNil(t, provider)

	_, ok := provider.(*SystemTTS)
	assert.True(t, ok, "expected *SystemTTS as default")
}

func TestNewTTSProvider_Unknown(t *testing.T) {
	provider := NewTTSProvider("unknown-provider", "")
	assert.NotNil(t, provider)

	_, ok := provider.(*SystemTTS)
	assert.True(t, ok, "expected *SystemTTS for unknown provider")
}
