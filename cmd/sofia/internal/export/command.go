package export

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/grasberg/sofia/cmd/sofia/internal"
	"github.com/grasberg/sofia/pkg/memory"
	"github.com/grasberg/sofia/pkg/providers"
)

// ---------------------------------------------------------------------------
// Export/import data types
// ---------------------------------------------------------------------------

// ExportData is the top-level envelope written to/read from JSON.
type ExportData struct {
	Version   int              `json:"version"`
	ExportedAt time.Time       `json:"exported_at"`
	Sessions  []ExportSession  `json:"sessions"`
	Memory    []ExportMemory   `json:"memory,omitempty"`
}

// ExportSession holds a single conversation session with its messages.
type ExportSession struct {
	Key       string              `json:"key"`
	AgentID   string              `json:"agent_id,omitempty"`
	Summary   string              `json:"summary,omitempty"`
	Messages  []providers.Message `json:"messages"`
	CreatedAt time.Time           `json:"created_at"`
	Metadata  ExportMetadata      `json:"metadata"`
}

// ExportMetadata carries auxiliary info about a session.
type ExportMetadata struct {
	MessageCount int    `json:"message_count"`
	Preview      string `json:"preview,omitempty"`
}

// ExportMemory represents a single memory note.
type ExportMemory struct {
	AgentID   string    `json:"agent_id"`
	Kind      string    `json:"kind"`
	DateKey   string    `json:"date_key"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ---------------------------------------------------------------------------
// Parent "data" command
// ---------------------------------------------------------------------------

// NewDataCommand returns the "sofia data" parent command with export/import subcommands.
func NewDataCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "data",
		Short: "Export and import session data",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}

	cmd.AddCommand(
		newExportCommand(),
		newImportCommand(),
	)

	return cmd
}

// ---------------------------------------------------------------------------
// "sofia data export"
// ---------------------------------------------------------------------------

func newExportCommand() *cobra.Command {
	var (
		sessionKey    string
		all           bool
		output        string
		includeMemory bool
	)

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export conversation sessions to JSON",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if sessionKey == "" && !all {
				return fmt.Errorf("specify --session <key> or --all")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			return runExport(db, sessionKey, all, includeMemory, output)
		},
	}

	cmd.Flags().StringVar(&sessionKey, "session", "", "Export a specific session by key")
	cmd.Flags().BoolVar(&all, "all", false, "Export all sessions")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output file path (default: stdout)")
	cmd.Flags().BoolVar(&includeMemory, "include-memory", false, "Also export agent memory notes")

	return cmd
}

// ---------------------------------------------------------------------------
// "sofia data import"
// ---------------------------------------------------------------------------

func newImportCommand() *cobra.Command {
	var input string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import conversation sessions from JSON",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if input == "" {
				return fmt.Errorf("--input <file> is required")
			}
			db, err := openDB()
			if err != nil {
				return err
			}
			defer db.Close()

			return runImport(db, input)
		},
	}

	cmd.Flags().StringVarP(&input, "input", "i", "", "Input JSON file to import")

	return cmd
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// openDB resolves the memory DB path from config and opens it.
func openDB() (*memory.MemoryDB, error) {
	cfg, err := internal.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("error loading config: %w", err)
	}
	dbPath := cfg.MemoryDB
	if dbPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("cannot determine home directory: %w", err)
		}
		dbPath = filepath.Join(home, ".sofia", "memory.db")
	}
	return memory.Open(dbPath)
}

// runExport builds the ExportData and writes it to output (file or stdout).
func runExport(db *memory.MemoryDB, sessionKey string, all, includeMemory bool, output string) error {
	data, err := buildExportData(db, sessionKey, all, includeMemory)
	if err != nil {
		return err
	}

	var w io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return fmt.Errorf("create output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(data)
}

// buildExportData is the testable core: it reads from the DB and returns the struct.
func buildExportData(
	db *memory.MemoryDB,
	sessionKey string,
	all, includeMemory bool,
) (*ExportData, error) {
	rows, err := db.ListSessions()
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}

	var sessions []ExportSession
	for _, r := range rows {
		if !all && r.Key != sessionKey {
			continue
		}

		msgs, err := db.GetMessages(r.Key)
		if err != nil {
			return nil, fmt.Errorf("get messages for %q: %w", r.Key, err)
		}
		if msgs == nil {
			msgs = []providers.Message{}
		}

		sessions = append(sessions, ExportSession{
			Key:       r.Key,
			AgentID:   r.AgentID,
			Summary:   r.Summary,
			Messages:  msgs,
			CreatedAt: r.CreatedAt,
			Metadata: ExportMetadata{
				MessageCount: r.MsgCount,
				Preview:      r.Preview,
			},
		})
	}

	if !all && len(sessions) == 0 {
		return nil, fmt.Errorf("session %q not found", sessionKey)
	}

	data := &ExportData{
		Version:    1,
		ExportedAt: time.Now().UTC(),
		Sessions:   sessions,
	}

	if includeMemory {
		notes, err := db.ListNotes()
		if err != nil {
			return nil, fmt.Errorf("list memory notes: %w", err)
		}
		for _, n := range notes {
			data.Memory = append(data.Memory, ExportMemory{
				AgentID:   n.AgentID,
				Kind:      n.Kind,
				DateKey:   n.DateKey,
				Content:   n.Content,
				UpdatedAt: n.UpdatedAt,
			})
		}
	}

	return data, nil
}

// runImport reads a JSON file and inserts sessions (and memory notes) into the DB.
func runImport(db *memory.MemoryDB, inputPath string) error {
	f, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("open input file: %w", err)
	}
	defer f.Close()

	var data ExportData
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}

	return importData(db, &data)
}

// importData is the testable core for importing.
func importData(db *memory.MemoryDB, data *ExportData) error {
	imported, skipped := 0, 0

	for _, s := range data.Sessions {
		// Check if session already exists — skip duplicates by session key.
		existing, _ := db.ListSessions()
		found := false
		for _, e := range existing {
			if e.Key == s.Key {
				found = true
				break
			}
		}
		if found {
			skipped++
			continue
		}

		agentID := s.AgentID
		if agentID == "" {
			agentID = "imported"
		}
		if _, err := db.GetOrCreateSession(s.Key, agentID); err != nil {
			return fmt.Errorf("create session %q: %w", s.Key, err)
		}
		if err := db.SetMessages(s.Key, s.Messages); err != nil {
			return fmt.Errorf("set messages for %q: %w", s.Key, err)
		}
		if s.Summary != "" {
			if err := db.SetSummary(s.Key, s.Summary); err != nil {
				return fmt.Errorf("set summary for %q: %w", s.Key, err)
			}
		}
		imported++
	}

	// Import memory notes (upsert, so duplicates are harmlessly overwritten).
	memImported := 0
	for _, m := range data.Memory {
		if err := db.SetNote(m.AgentID, m.Kind, m.DateKey, m.Content); err != nil {
			return fmt.Errorf("set note: %w", err)
		}
		memImported++
	}

	fmt.Fprintf(os.Stderr, "imported %d session(s), skipped %d duplicate(s)", imported, skipped)
	if memImported > 0 {
		fmt.Fprintf(os.Stderr, ", imported %d memory note(s)", memImported)
	}
	fmt.Fprintln(os.Stderr)

	return nil
}
