package channels

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// mockRunner captures calls and returns scripted responses.
type mockRunner struct {
	calls     [][]string
	responses map[string]mockResponse
}

type mockResponse struct {
	out []byte
	err error
}

func (m *mockRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	m.calls = append(m.calls, append([]string(nil), args...))
	key := strings.Join(args, " ")
	if resp, ok := m.responses[key]; ok {
		return resp.out, resp.err
	}
	for pattern, resp := range m.responses {
		if strings.Contains(key, pattern) {
			return resp.out, resp.err
		}
	}
	return nil, fmt.Errorf("mockRunner: no scripted response for %q", key)
}

func encodeBody(s string) string {
	return base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString([]byte(s))
}

func TestParseMessageIDs_Envelope(t *testing.T) {
	out := []byte(`{"messages":[{"id":"m1","threadId":"t1"},{"id":"m2"}]}`)
	ids, err := parseMessageIDs(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 || ids[0] != "m1" || ids[1] != "m2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestParseMessageIDs_BareArray(t *testing.T) {
	out := []byte(`[{"id":"a"},{"id":"b"},{"id":"c"}]`)
	ids, err := parseMessageIDs(out)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 3 {
		t.Fatalf("want 3 ids, got %v", ids)
	}
}

func TestParseMessageIDs_Empty(t *testing.T) {
	ids, err := parseMessageIDs([]byte(""))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 0 {
		t.Fatalf("want empty, got %v", ids)
	}
}

func TestParseMessageIDs_UnknownShape(t *testing.T) {
	_, err := parseMessageIDs([]byte(`{"weird":"nope"}`))
	if err == nil {
		t.Fatal("expected error for unrecognized shape")
	}
}

func TestParseGmailMessage_PlainText(t *testing.T) {
	raw := fmt.Sprintf(`{
        "id":"abc",
        "threadId":"t1",
        "internalDate":"1700000000000",
        "payload":{
            "mimeType":"text/plain",
            "headers":[
                {"name":"From","value":"Alice <alice@example.com>"},
                {"name":"Subject","value":"Help"},
                {"name":"Message-ID","value":"<msg@example.com>"}
            ],
            "body":{"data":"%s"}
        }
    }`, encodeBody("Hello, world"))

	email, err := parseGmailMessage([]byte(raw), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email == nil {
		t.Fatal("expected email, got nil")
	}
	if email.From != "alice@example.com" {
		t.Errorf("From = %q, want alice@example.com", email.From)
	}
	if email.Subject != "Help" {
		t.Errorf("Subject = %q", email.Subject)
	}
	if email.Body != "Hello, world" {
		t.Errorf("Body = %q", email.Body)
	}
	if email.MessageID != "<msg@example.com>" {
		t.Errorf("MessageID = %q", email.MessageID)
	}
	if email.Date.IsZero() {
		t.Error("Date should be parsed from internalDate")
	}
}

func TestParseGmailMessage_MultipartPrefersTextPlain(t *testing.T) {
	raw := fmt.Sprintf(`{
        "id":"m",
        "payload":{
            "mimeType":"multipart/alternative",
            "headers":[{"name":"From","value":"bob@example.com"}],
            "parts":[
                {"mimeType":"text/html","body":{"data":"%s"}},
                {"mimeType":"text/plain","body":{"data":"%s"}}
            ]
        }
    }`, encodeBody("<b>HTML</b>"), encodeBody("Plain body"))

	email, err := parseGmailMessage([]byte(raw), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email.Body != "Plain body" {
		t.Errorf("Body = %q, want Plain body", email.Body)
	}
}

func TestParseGmailMessage_TruncatesLargeBody(t *testing.T) {
	body := strings.Repeat("x", 1000)
	raw := fmt.Sprintf(`{
        "id":"m",
        "payload":{"mimeType":"text/plain","body":{"data":"%s"},"headers":[]}
    }`, encodeBody(body))

	email, err := parseGmailMessage([]byte(raw), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(email.Body, "[truncated]") {
		t.Errorf("expected truncation marker, got %q…", email.Body[len(email.Body)-30:])
	}
}

func TestGmailReceiver_PollHappyPath(t *testing.T) {
	msg1 := fmt.Sprintf(`{"id":"m1","payload":{"mimeType":"text/plain","headers":[{"name":"From","value":"a@x.com"},{"name":"Subject","value":"s1"}],"body":{"data":"%s"}}}`, encodeBody("body-one"))
	msg2 := fmt.Sprintf(`{"id":"m2","payload":{"mimeType":"text/plain","headers":[{"name":"From","value":"b@x.com"},{"name":"Subject","value":"s2"}],"body":{"data":"%s"}}}`, encodeBody("body-two"))

	runner := &mockRunner{responses: map[string]mockResponse{
		"messages search": {out: []byte(`{"messages":[{"id":"m1"},{"id":"m2"}]}`)},
		"gmail get m1":    {out: []byte(msg1)},
		"gmail get m2":    {out: []byte(msg2)},
	}}
	r := newGmailReceiver(runner, GmailReceiverOptions{Query: "is:unread", MaxPerPoll: 5})

	emails, err := r.Poll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(emails) != 2 {
		t.Fatalf("want 2 emails, got %d", len(emails))
	}
	if emails[0].Body != "body-one" || emails[1].Body != "body-two" {
		t.Errorf("unexpected bodies: %+v", emails)
	}
}

func TestGmailReceiver_SearchError(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{
		"messages search": {err: errors.New("gog oauth expired")},
	}}
	r := newGmailReceiver(runner, GmailReceiverOptions{})

	_, err := r.Poll(context.Background())
	if err == nil || !strings.Contains(err.Error(), "oauth expired") {
		t.Fatalf("expected error propagation, got %v", err)
	}
}

func TestGmailReceiver_SkipsFailedFetches(t *testing.T) {
	msg2 := fmt.Sprintf(`{"id":"m2","payload":{"mimeType":"text/plain","headers":[],"body":{"data":"%s"}}}`, encodeBody("ok"))
	runner := &mockRunner{responses: map[string]mockResponse{
		"messages search": {out: []byte(`{"messages":[{"id":"m1"},{"id":"m2"}]}`)},
		"gmail get m1":    {err: errors.New("403")},
		"gmail get m2":    {out: []byte(msg2)},
	}}
	r := newGmailReceiver(runner, GmailReceiverOptions{})

	emails, err := r.Poll(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(emails) != 1 || emails[0].Body != "ok" {
		t.Fatalf("want [ok], got %+v", emails)
	}
}

func TestGmailReceiver_MarkAsReadInvokesModify(t *testing.T) {
	msg := fmt.Sprintf(`{"id":"m1","payload":{"mimeType":"text/plain","headers":[],"body":{"data":"%s"}}}`, encodeBody("x"))
	runner := &mockRunner{responses: map[string]mockResponse{
		"messages search":         {out: []byte(`{"messages":[{"id":"m1"}]}`)},
		"gmail get m1":            {out: []byte(msg)},
		"messages modify m1":      {out: []byte(`{"ok":true}`)},
	}}
	r := newGmailReceiver(runner, GmailReceiverOptions{MarkAsRead: true})

	if _, err := r.Poll(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sawModify := false
	for _, c := range runner.calls {
		joined := strings.Join(c, " ")
		if strings.Contains(joined, "messages modify m1") && strings.Contains(joined, "UNREAD") {
			sawModify = true
		}
	}
	if !sawModify {
		t.Errorf("expected a messages-modify call removing UNREAD, calls=%v", runner.calls)
	}
}

func TestExtractEmailAddress(t *testing.T) {
	cases := map[string]string{
		"alice@example.com":           "alice@example.com",
		"Alice <alice@example.com>":   "alice@example.com",
		"\"A. Person\" <a@p.se>":      "a@p.se",
		"  bob@example.com  ":         "bob@example.com",
		"":                            "",
	}
	for in, want := range cases {
		if got := extractEmailAddress(in); got != want {
			t.Errorf("extractEmailAddress(%q) = %q, want %q", in, got, want)
		}
	}
}
