package memory

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// KBEntry is a knowledge-base item the agent can retrieve when drafting a
// reply. It is stored as a semantic_nodes row with label="KBEntry" — this
// piggybacks on the existing storage and indexing machinery without a new
// table.
type KBEntry struct {
	ID         int64     `json:"id"`
	AgentID    string    `json:"agent_id"`
	Question   string    `json:"question"`
	Answer     string    `json:"answer"`
	Source     string    `json:"source,omitempty"`
	Tags       []string  `json:"tags,omitempty"`
	ReplyCount int       `json:"reply_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// kbProps is the on-disk JSON shape inside semantic_nodes.properties.
type kbProps struct {
	Question   string   `json:"question"`
	Answer     string   `json:"answer"`
	Source     string   `json:"source,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	ReplyCount int      `json:"reply_count,omitempty"`
}

const kbLabel = "KBEntry"

// UpsertKBEntry inserts or updates a KB entry keyed by a stable hash of the
// question so repeated drafts for the same inbound question merge.
func (m *MemoryDB) UpsertKBEntry(agentID, question, answer, source string, tags []string) (int64, error) {
	question = strings.TrimSpace(question)
	answer = strings.TrimSpace(answer)
	if question == "" || answer == "" {
		return 0, fmt.Errorf("memory: kb upsert requires non-empty question and answer")
	}

	key := kbEntryKey(question)

	existing, _ := m.GetKBEntryByKey(agentID, key)
	count := 1
	if existing != nil {
		count = existing.ReplyCount + 1
	}

	props := kbProps{
		Question:   question,
		Answer:     answer,
		Source:     source,
		Tags:       normalizeTags(tags),
		ReplyCount: count,
	}
	b, err := json.Marshal(props)
	if err != nil {
		return 0, fmt.Errorf("memory: kb props marshal: %w", err)
	}

	id, err := m.UpsertNode(agentID, kbLabel, key, string(b))
	if err != nil {
		return 0, fmt.Errorf("memory: kb upsert: %w", err)
	}
	return id, nil
}

// GetKBEntryByKey returns the KBEntry whose node name equals the question
// hash, or nil if not found.
func (m *MemoryDB) GetKBEntryByKey(agentID, key string) (*KBEntry, error) {
	nodes, err := m.FindNodes(agentID, kbLabel, key, 1)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}
	e, err := nodeToKBEntry(nodes[0])
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// SearchKBEntries returns up to topK entries ranked by token-overlap with the
// query. Agent-scoped when agentID is non-empty; empty string searches across
// all agents — useful when a drafting subagent shares its owner's KB.
func (m *MemoryDB) SearchKBEntries(agentID, query string, topK int) ([]KBEntry, error) {
	query = strings.TrimSpace(query)
	if query == "" || topK <= 0 {
		return nil, nil
	}

	var nodes []SemanticNode
	var err error
	if agentID == "" {
		nodes, err = m.FindNodesByLabel(kbLabel, 0)
	} else {
		nodes, err = m.FindNodes(agentID, kbLabel, "%", 0)
	}
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, nil
	}

	queryTokens := tokenize(query)
	if len(queryTokens) == 0 {
		return nil, nil
	}

	type scored struct {
		entry KBEntry
		score float64
	}
	scoredEntries := make([]scored, 0, len(nodes))
	for _, n := range nodes {
		entry, err := nodeToKBEntry(n)
		if err != nil {
			continue
		}
		sc := score(queryTokens, entry)
		if sc > 0 {
			scoredEntries = append(scoredEntries, scored{entry: entry, score: sc})
		}
	}

	sort.SliceStable(scoredEntries, func(i, j int) bool {
		return scoredEntries[i].score > scoredEntries[j].score
	})

	if topK > len(scoredEntries) {
		topK = len(scoredEntries)
	}
	out := make([]KBEntry, 0, topK)
	for i := 0; i < topK; i++ {
		out = append(out, scoredEntries[i].entry)
	}
	return out, nil
}

// DeleteKBEntry removes a KB entry by ID.
func (m *MemoryDB) DeleteKBEntry(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, err := m.db.Exec(`DELETE FROM semantic_nodes WHERE id = ? AND label = ?`, id, kbLabel)
	if err != nil {
		return fmt.Errorf("memory: kb delete: %w", err)
	}
	return nil
}

// kbEntryKey produces a stable short key from a question string. We normalize
// (lowercase, collapse whitespace, strip punctuation) before hashing so
// slightly rephrased repeat questions still merge.
func kbEntryKey(question string) string {
	norm := strings.Map(func(r rune) rune {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r):
			return unicode.ToLower(r)
		case unicode.IsSpace(r):
			return ' '
		default:
			return -1
		}
	}, question)
	norm = strings.Join(strings.Fields(norm), " ")
	sum := sha1.Sum([]byte(norm))
	return "kb-" + hex.EncodeToString(sum[:8])
}

// nodeToKBEntry decodes the JSON props back into a KBEntry.
func nodeToKBEntry(n SemanticNode) (KBEntry, error) {
	var props kbProps
	if err := json.Unmarshal([]byte(n.Properties), &props); err != nil {
		return KBEntry{}, fmt.Errorf("kb entry parse: %w", err)
	}
	return KBEntry{
		ID:         n.ID,
		AgentID:    n.AgentID,
		Question:   props.Question,
		Answer:     props.Answer,
		Source:     props.Source,
		Tags:       props.Tags,
		ReplyCount: props.ReplyCount,
		CreatedAt:  n.CreatedAt,
		UpdatedAt:  n.UpdatedAt,
	}, nil
}

// tokenize splits text into lowercased alphanumeric tokens and drops common
// stopwords so the overlap score isn't dominated by noise.
func tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		if len(f) < 2 {
			continue
		}
		if _, stop := stopwords[f]; stop {
			continue
		}
		out = append(out, f)
	}
	return out
}

// score ranks an entry by how many query tokens appear in its question or
// answer. A token match in the question scores double. Tags get a small
// boost. Reply-count log-dampens to prefer frequently-used entries.
func score(queryTokens []string, e KBEntry) float64 {
	questionTokens := tokenize(e.Question)
	answerTokens := tokenize(e.Answer)

	questionSet := setOf(questionTokens)
	answerSet := setOf(answerTokens)
	tagSet := make(map[string]struct{}, len(e.Tags))
	for _, t := range e.Tags {
		tagSet[strings.ToLower(t)] = struct{}{}
	}

	var s float64
	for _, q := range queryTokens {
		if _, ok := questionSet[q]; ok {
			s += 2.0
		}
		if _, ok := answerSet[q]; ok {
			s += 1.0
		}
		if _, ok := tagSet[q]; ok {
			s += 1.5
		}
	}

	// Logarithmic popularity bonus — keeps low/zero-reply entries in the
	// mix while nudging proven ones upward.
	if e.ReplyCount > 0 {
		s += 0.2 * float64(e.ReplyCount)
	}
	return s
}

func setOf(tokens []string) map[string]struct{} {
	out := make(map[string]struct{}, len(tokens))
	for _, t := range tokens {
		out[t] = struct{}{}
	}
	return out
}

func normalizeTags(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, t := range in {
		t = strings.TrimSpace(strings.ToLower(t))
		if t == "" {
			continue
		}
		if _, dup := seen[t]; dup {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

// stopwords: tiny built-in EN+SV list — enough to clean query noise without
// dragging in a full NLP library.
var stopwords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "are": {}, "but": {},
	"not": {}, "you": {}, "with": {}, "your": {}, "can": {},
	"this": {}, "that": {}, "from": {}, "have": {}, "was": {},
	"och": {}, "att": {}, "det": {}, "som": {}, "jag": {},
	"har": {}, "inte": {}, "men": {}, "den": {}, "med": {},
}
