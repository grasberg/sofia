package eval

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadBuiltinSuites(t *testing.T) {
	suites, err := LoadBuiltinSuites()
	require.NoError(t, err)
	require.Len(t, suites, 5, "expected 5 embedded benchmark suites")

	names := make(map[string]bool)
	for _, s := range suites {
		names[s.Name] = true
		assert.NotEmpty(t, s.Description, "suite %q should have a description", s.Name)
		assert.NotEmpty(t, s.Cases, "suite %q should have test cases", s.Name)
	}

	assert.True(t, names["tool_use"], "missing tool_use suite")
	assert.True(t, names["reasoning"], "missing reasoning suite")
	assert.True(t, names["guardrails"], "missing guardrails suite")
	assert.True(t, names["delegation"], "missing delegation suite")
	assert.True(t, names["general"], "missing general suite")
}

func TestLoadBuiltinSuite_Individual(t *testing.T) {
	tests := []struct {
		name     string
		minCases int
	}{
		{"tool_use", 8},
		{"reasoning", 6},
		{"guardrails", 8},
		{"delegation", 5},
		{"general", 6},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite, err := LoadBuiltinSuite(tt.name)
			require.NoError(t, err)

			assert.Equal(t, tt.name, suite.Name)
			assert.GreaterOrEqual(t, len(suite.Cases), tt.minCases,
				"suite %q should have at least %d cases", tt.name, tt.minCases)

			// Verify all cases have required fields.
			for i, tc := range suite.Cases {
				assert.NotEmpty(t, tc.Name, "case %d in suite %q missing name", i, tt.name)
				assert.NotEmpty(t, tc.Input, "case %d in suite %q missing input", i, tt.name)
				assert.NotEmpty(t, tc.Tags, "case %d in suite %q missing tags", i, tt.name)
			}
		})
	}
}

func TestLoadBuiltinSuite_NotFound(t *testing.T) {
	_, err := LoadBuiltinSuite("nonexistent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read embedded benchmark")
}

func TestBuiltinSuites_CasesAreValid(t *testing.T) {
	suites, err := LoadBuiltinSuites()
	require.NoError(t, err)

	runner := NewEvalRunner()

	for _, s := range suites {
		t.Run(s.Name, func(t *testing.T) {
			for _, tc := range s.Cases {
				// Verify the test case can be executed without panicking.
				result := runner.RunTest(tc, "test output", 0)
				assert.NotEmpty(t, result.Name)
			}
		})
	}
}
