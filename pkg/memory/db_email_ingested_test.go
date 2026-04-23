package memory

import (
	"testing"
)

func TestEmailIngested_MarkAndCheck(t *testing.T) {
	db := openTestDB(t)

	seen, err := db.IsEmailIngested("m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen {
		t.Error("brand-new store should report unseen")
	}

	if err := db.MarkEmailIngested("m1", "t1", "alice@example.com", "hi"); err != nil {
		t.Fatalf("mark: %v", err)
	}

	seen, err = db.IsEmailIngested("m1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !seen {
		t.Error("expected m1 to be marked as seen")
	}
}

func TestEmailIngested_DuplicateMarkIsSilent(t *testing.T) {
	db := openTestDB(t)

	if err := db.MarkEmailIngested("m1", "", "", ""); err != nil {
		t.Fatalf("first mark: %v", err)
	}
	if err := db.MarkEmailIngested("m1", "", "", ""); err != nil {
		t.Fatalf("second mark should be idempotent: %v", err)
	}
}

func TestEmailIngested_EmptyIDIsUnseen(t *testing.T) {
	db := openTestDB(t)
	seen, err := db.IsEmailIngested("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if seen {
		t.Error("empty message ID must never be reported as seen")
	}
}

func TestEmailIngested_MarkEmptyIDFails(t *testing.T) {
	db := openTestDB(t)
	if err := db.MarkEmailIngested("", "", "", ""); err == nil {
		t.Error("marking empty ID should fail loudly")
	}
}

func TestEmailIngested_Prune(t *testing.T) {
	db := openTestDB(t)

	if err := db.MarkEmailIngested("old", "", "", ""); err != nil {
		t.Fatalf("mark: %v", err)
	}
	// Force the row's ingested_at far into the past so the default window hits it.
	if _, err := db.db.Exec(`UPDATE email_ingested SET ingested_at = datetime('now','-90 days') WHERE message_id='old'`); err != nil {
		t.Fatalf("age row: %v", err)
	}
	if err := db.MarkEmailIngested("fresh", "", "", ""); err != nil {
		t.Fatalf("mark fresh: %v", err)
	}

	n, err := db.PruneEmailIngestedBefore("-30 days")
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if n != 1 {
		t.Errorf("want 1 row pruned, got %d", n)
	}

	seen, _ := db.IsEmailIngested("old")
	if seen {
		t.Error("old row should be gone")
	}
	seen, _ = db.IsEmailIngested("fresh")
	if !seen {
		t.Error("fresh row should survive")
	}
}
