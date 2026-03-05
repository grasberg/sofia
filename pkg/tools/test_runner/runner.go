package test_runner

import (
	"bytes"
	"os/exec"
)

// TestResult representerar resultatet av en go test körning
type TestResult struct {
	Passed bool
	Output string
	Errors []string
}

// RunTests kör 'go test ./...' och returnerar strukturerad data
func RunTests() (*TestResult, error) {
	cmd := exec.Command("go", "test", "./...", "-json")
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()

	result := &TestResult{
		Passed: err == nil,
		Output: out.String(),
	}

	// Här skulle vi kunna lägga till logik för att parsa JSON-output
	// och extrahera specifika felmeddelanden.

	return result, nil
}
