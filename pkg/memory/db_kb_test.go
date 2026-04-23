package memory

import (
	"testing"
)

func TestUpsertKBEntry_StoresAndRetrieves(t *testing.T) {
	db := openTestDB(t)

	id, err := db.UpsertKBEntry("agent-1", "How do I reset my password?",
		"Visit settings → Security → Reset.", "email:alice@example.com",
		[]string{"account", "password"})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}

	entry, err := db.GetKBEntryByKey("agent-1", kbEntryKey("How do I reset my password?"))
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if entry == nil {
		t.Fatal("entry should exist")
	}
	if entry.Answer != "Visit settings → Security → Reset." {
		t.Errorf("answer = %q", entry.Answer)
	}
	if entry.ReplyCount != 1 {
		t.Errorf("reply count = %d, want 1", entry.ReplyCount)
	}
	if len(entry.Tags) != 2 {
		t.Errorf("tags = %v", entry.Tags)
	}
}

func TestUpsertKBEntry_IncrementsReplyCountOnRepeat(t *testing.T) {
	db := openTestDB(t)

	q := "How do I cancel my subscription?"
	if _, err := db.UpsertKBEntry("a", q, "Visit billing → Cancel.", "", nil); err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpsertKBEntry("a", q, "Visit billing → Cancel.", "", nil); err != nil {
		t.Fatal(err)
	}

	entry, _ := db.GetKBEntryByKey("a", kbEntryKey(q))
	if entry == nil || entry.ReplyCount != 2 {
		t.Errorf("want reply count 2, got %+v", entry)
	}
}

func TestUpsertKBEntry_NormalizedKeyMatchesParaphrase(t *testing.T) {
	db := openTestDB(t)

	if _, err := db.UpsertKBEntry("a", "  How do  I reset my Password?? ", "ans", "", nil); err != nil {
		t.Fatal(err)
	}
	entry, _ := db.GetKBEntryByKey("a", kbEntryKey("how do i reset my password"))
	if entry == nil {
		t.Error("case/whitespace/punct differences should still match")
	}
}

func TestUpsertKBEntry_RejectsEmpty(t *testing.T) {
	db := openTestDB(t)
	if _, err := db.UpsertKBEntry("a", "", "ans", "", nil); err == nil {
		t.Error("empty question should fail")
	}
	if _, err := db.UpsertKBEntry("a", "q", "", "", nil); err == nil {
		t.Error("empty answer should fail")
	}
}

func TestSearchKBEntries_RanksByOverlap(t *testing.T) {
	db := openTestDB(t)

	_, _ = db.UpsertKBEntry("a", "How to reset password", "Settings → Security", "", []string{"password"})
	_, _ = db.UpsertKBEntry("a", "Refund policy", "We refund within 14 days", "", []string{"billing", "refund"})
	_, _ = db.UpsertKBEntry("a", "How to change billing address", "Go to profile → address", "", []string{"billing"})

	hits, err := db.SearchKBEntries("a", "password", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 || hits[0].Question != "How to reset password" {
		t.Errorf("password query didn't rank password answer first: %+v", hits)
	}

	hits, err = db.SearchKBEntries("a", "refund", 3)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(hits) == 0 || hits[0].Question != "Refund policy" {
		t.Errorf("refund query didn't rank refund answer first: %+v", hits)
	}
}

func TestSearchKBEntries_TopKBound(t *testing.T) {
	db := openTestDB(t)

	for i := 0; i < 5; i++ {
		_, _ = db.UpsertKBEntry("a", "question "+string(rune('a'+i))+" password",
			"answer", "", nil)
	}
	hits, _ := db.SearchKBEntries("a", "password", 2)
	if len(hits) != 2 {
		t.Errorf("want 2 hits, got %d", len(hits))
	}
}

func TestSearchKBEntries_ReplyCountBoost(t *testing.T) {
	db := openTestDB(t)

	_, _ = db.UpsertKBEntry("a", "How to contact support", "Email us", "", nil)

	// Second entry has the same topical match but higher reply count.
	for i := 0; i < 3; i++ {
		_, _ = db.UpsertKBEntry("a", "How to contact the team", "Slack us", "", nil)
	}

	hits, _ := db.SearchKBEntries("a", "contact", 2)
	if len(hits) < 1 {
		t.Fatal("need at least one hit")
	}
	// Top result should be the more-used "contact the team" answer.
	if hits[0].Question != "How to contact the team" {
		t.Errorf("reply-count boost not applied: top = %q", hits[0].Question)
	}
}

func TestSearchKBEntries_EmptyQuery(t *testing.T) {
	db := openTestDB(t)
	_, _ = db.UpsertKBEntry("a", "q", "ans", "", nil)

	hits, err := db.SearchKBEntries("a", "", 5)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(hits) != 0 {
		t.Error("empty query should return no hits")
	}
}

func TestDeleteKBEntry(t *testing.T) {
	db := openTestDB(t)
	id, _ := db.UpsertKBEntry("a", "q1", "a1", "", nil)

	if err := db.DeleteKBEntry(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	entry, _ := db.GetKBEntryByKey("a", kbEntryKey("q1"))
	if entry != nil {
		t.Error("entry should be gone after delete")
	}
}

func TestTokenize_SkipsStopwords(t *testing.T) {
	tokens := tokenize("The user has a password reset request")
	for _, tok := range tokens {
		if _, stop := stopwords[tok]; stop {
			t.Errorf("stopword %q should have been removed", tok)
		}
	}
	if len(tokens) == 0 {
		t.Fatal("should have non-stopword tokens")
	}
}
