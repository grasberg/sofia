package remote

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTailscaleManager(t *testing.T) {
	tm := NewTailscaleManager()
	require.NotNil(t, tm)
}

func TestTailscaleManager_IsAvailable(t *testing.T) {
	tm := NewTailscaleManager()
	// Just verify it returns a bool without panicking.
	_ = tm.IsAvailable()
}

func TestTailscaleManager_ParseStatus(t *testing.T) {
	sampleJSON := `{
		"BackendState": "Running",
		"Self": {
			"DNSName": "myhost.tail1234.ts.net.",
			"TailscaleIPs": ["100.64.0.1", "fd7a:115c:a1e0::1"]
		}
	}`

	var status TailscaleStatus
	err := json.Unmarshal([]byte(sampleJSON), &status)
	require.NoError(t, err)

	assert.Equal(t, "Running", status.BackendState)
	assert.Equal(t, "myhost.tail1234.ts.net.", status.Self.DNSName)
	assert.Equal(t, []string{"100.64.0.1", "fd7a:115c:a1e0::1"}, status.Self.TailscaleIPs)
}
