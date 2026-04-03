package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TestSuite groups related test cases for evaluation.
type TestSuite struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	AgentID     string     `json:"agent_id,omitempty"`
	Model       string     `json:"model,omitempty"`
	Cases       []TestCase `json:"cases"`
}

// LoadSuite reads and validates a single test suite from a JSON file.
func LoadSuite(path string) (*TestSuite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read suite file %q: %w", path, err)
	}

	var suite TestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("parse suite %q: %w", path, err)
	}

	if err := validateSuite(&suite, path); err != nil {
		return nil, err
	}

	return &suite, nil
}

// LoadSuitesFromDir loads all .json files in a directory as test suites.
// Non-JSON files are silently ignored. Returns an error if the directory
// cannot be read or if any JSON file fails to parse/validate.
func LoadSuitesFromDir(dir string) ([]*TestSuite, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read suite directory %q: %w", dir, err)
	}

	var suites []*TestSuite

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !strings.HasSuffix(strings.ToLower(entry.Name()), ".json") {
			continue
		}

		path := filepath.Join(dir, entry.Name())

		suite, err := LoadSuite(path)
		if err != nil {
			return nil, fmt.Errorf("load suite %q: %w", entry.Name(), err)
		}

		suites = append(suites, suite)
	}

	return suites, nil
}

// validateSuite checks that required fields are present on the suite and its cases.
func validateSuite(suite *TestSuite, path string) error {
	if suite.Name == "" {
		return fmt.Errorf("suite %q: missing required field \"name\"", path)
	}

	if len(suite.Cases) == 0 {
		return fmt.Errorf("suite %q: no test cases defined", path)
	}

	for i, tc := range suite.Cases {
		if tc.Name == "" {
			return fmt.Errorf("suite %q: case %d missing required field \"name\"", path, i)
		}

		if tc.Input == "" {
			return fmt.Errorf("suite %q: case %q missing required field \"input\"", path, tc.Name)
		}
	}

	return nil
}
