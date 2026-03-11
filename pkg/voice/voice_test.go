package voice

import (
	"testing"
	"time"
)

func TestNewGroqTranscriber(t *testing.T) {
	apiKey := "test-key-123"
	transcriber := NewGroqTranscriber(apiKey)

	if transcriber == nil {
		t.Fatal("NewGroqTranscriber returned nil")
	}
	if transcriber.apiKey != apiKey {
		t.Errorf("apiKey mismatch: got %q, want %q", transcriber.apiKey, apiKey)
	}
	if transcriber.apiBase != "https://api.groq.com/openai/v1" {
		t.Errorf("apiBase mismatch: got %q, want %q", transcriber.apiBase, "https://api.groq.com/openai/v1")
	}
	if transcriber.httpClient == nil {
		t.Fatal("httpClient is nil")
	}
	if transcriber.httpClient.Timeout != 60*time.Second {
		t.Errorf("httpClient timeout mismatch: got %v, want %v", transcriber.httpClient.Timeout, 60*time.Second)
	}
}

func TestNewGroqTranscriberEmptyKey(t *testing.T) {
	transcriber := NewGroqTranscriber("")

	if transcriber == nil {
		t.Fatal("NewGroqTranscriber returned nil")
	}
	if transcriber.apiKey != "" {
		t.Errorf("apiKey should be empty, got %q", transcriber.apiKey)
	}
}

func TestIsAvailableWithKey(t *testing.T) {
	transcriber := NewGroqTranscriber("valid-key")
	if !transcriber.IsAvailable() {
		t.Error("IsAvailable should return true when apiKey is set")
	}
}

func TestIsAvailableWithoutKey(t *testing.T) {
	transcriber := NewGroqTranscriber("")
	if transcriber.IsAvailable() {
		t.Error("IsAvailable should return false when apiKey is empty")
	}
}

func TestTranscriptionResponse_Unmarshal(t *testing.T) {
	resp := TranscriptionResponse{
		Text:     "Hello, world!",
		Language: "en",
		Duration: 2.5,
	}

	if resp.Text != "Hello, world!" {
		t.Errorf("Text mismatch: got %q, want %q", resp.Text, "Hello, world!")
	}
	if resp.Language != "en" {
		t.Errorf("Language mismatch: got %q, want %q", resp.Language, "en")
	}
	if resp.Duration != 2.5 {
		t.Errorf("Duration mismatch: got %v, want %v", resp.Duration, 2.5)
	}
}

func TestTranscriptionResponseEmpty(t *testing.T) {
	resp := TranscriptionResponse{}

	if resp.Text != "" {
		t.Errorf("Text should be empty, got %q", resp.Text)
	}
	if resp.Language != "" {
		t.Errorf("Language should be empty, got %q", resp.Language)
	}
	if resp.Duration != 0 {
		t.Errorf("Duration should be 0, got %v", resp.Duration)
	}
}

func TestGroqTranscriberMultipleInstances(t *testing.T) {
	t1 := NewGroqTranscriber("key1")
	t2 := NewGroqTranscriber("key2")

	if t1.apiKey == t2.apiKey {
		t.Error("Different instances should have different API keys")
	}
	if t1.httpClient == t2.httpClient {
		t.Error("Different instances should have different HTTP clients")
	}
}

func TestGroqTranscriberAPIBase(t *testing.T) {
	transcriber := NewGroqTranscriber("test-key")

	expectedBase := "https://api.groq.com/openai/v1"
	if transcriber.apiBase != expectedBase {
		t.Errorf("apiBase mismatch: got %q, want %q", transcriber.apiBase, expectedBase)
	}
}

func TestGroqTranscriberHTTPClientTimeout(t *testing.T) {
	transcriber := NewGroqTranscriber("test-key")

	if transcriber.httpClient.Timeout != 60*time.Second {
		t.Errorf("Timeout mismatch: got %v, want 60s", transcriber.httpClient.Timeout)
	}
}
