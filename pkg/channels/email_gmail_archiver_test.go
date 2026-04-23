package channels

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGmailArchiver_NoopOnEmptyID(t *testing.T) {
	a := newGmailArchiver(&mockRunner{responses: map[string]mockResponse{}}, GmailArchiverOptions{MarkRead: true})
	if err := a.Archive(context.Background(), ""); err != nil {
		t.Errorf("empty id should be silent no-op, got %v", err)
	}
}

func TestGmailArchiver_NoopWhenNothingToDo(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{}}
	a := newGmailArchiver(runner, GmailArchiverOptions{})
	if err := a.Archive(context.Background(), "msg-1"); err != nil {
		t.Errorf("nothing-to-do should be no-op, got %v", err)
	}
	if len(runner.calls) != 0 {
		t.Errorf("expected no gog calls, got %d", len(runner.calls))
	}
}

func TestGmailArchiver_MarkReadOnly(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{
		"messages modify": {out: []byte(`{"ok":true}`)},
	}}
	a := newGmailArchiver(runner, GmailArchiverOptions{MarkRead: true})
	if err := a.Archive(context.Background(), "m1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %d", len(runner.calls))
	}
	joined := strings.Join(runner.calls[0], " ")
	if !strings.Contains(joined, "--remove-label UNREAD") {
		t.Errorf("missing remove-label flag: %s", joined)
	}
	if strings.Contains(joined, "--add-label") {
		t.Error("should not add label when AddLabel empty")
	}
}

func TestGmailArchiver_MarkReadAndAddLabel(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{
		"messages modify": {out: []byte(`{"ok":true}`)},
	}}
	a := newGmailArchiver(runner, GmailArchiverOptions{MarkRead: true, AddLabel: "handled"})
	if err := a.Archive(context.Background(), "m1"); err != nil {
		t.Fatalf("archive: %v", err)
	}
	joined := strings.Join(runner.calls[0], " ")
	if !strings.Contains(joined, "--add-label handled") {
		t.Errorf("missing add-label: %s", joined)
	}
}

func TestGmailArchiver_PropagatesRunnerError(t *testing.T) {
	runner := &mockRunner{responses: map[string]mockResponse{
		"messages modify": {err: errors.New("oauth expired")},
	}}
	a := newGmailArchiver(runner, GmailArchiverOptions{MarkRead: true})
	if err := a.Archive(context.Background(), "m1"); err == nil {
		t.Error("expected error from runner failure")
	}
}
