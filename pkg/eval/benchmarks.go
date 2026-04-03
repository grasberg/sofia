package eval

import (
	"embed"
	"encoding/json"
	"fmt"
	"strings"
)

//go:embed benchmarks/*.json
var BuiltinBenchmarks embed.FS

// LoadBuiltinSuites returns all embedded benchmark suites.
func LoadBuiltinSuites() ([]*TestSuite, error) {
	entries, err := BuiltinBenchmarks.ReadDir("benchmarks")
	if err != nil {
		return nil, fmt.Errorf("read embedded benchmarks: %w", err)
	}

	var suites []*TestSuite

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".json")

		suite, err := LoadBuiltinSuite(name)
		if err != nil {
			return nil, err
		}

		suites = append(suites, suite)
	}

	return suites, nil
}

// LoadBuiltinSuite loads a single embedded benchmark by name (e.g., "tool_use").
func LoadBuiltinSuite(name string) (*TestSuite, error) {
	path := "benchmarks/" + name + ".json"

	data, err := BuiltinBenchmarks.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read embedded benchmark %q: %w", name, err)
	}

	var suite TestSuite
	if err := json.Unmarshal(data, &suite); err != nil {
		return nil, fmt.Errorf("parse embedded benchmark %q: %w", name, err)
	}

	if err := validateSuite(&suite, path); err != nil {
		return nil, err
	}

	return &suite, nil
}
