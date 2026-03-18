package daemon

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateLaunchdPlist(t *testing.T) {
	binary := "/usr/local/bin/sofia"
	logDir := "/Users/test/.sofia/logs"

	plist := generateLaunchdPlist(binary, logDir)

	assert.Contains(t, plist, "<string>/usr/local/bin/sofia</string>")
	assert.Contains(t, plist, "<string>gateway</string>")
	assert.Contains(t, plist, "<string>com.sofia.gateway</string>")
	assert.Contains(t, plist, "<key>KeepAlive</key>")
	assert.Contains(t, plist, "<true/>")
	assert.Contains(t, plist, "<key>RunAtLoad</key>")
	assert.Contains(t, plist, logDir+"/sofia.log")
	assert.Contains(t, plist, logDir+"/sofia.err.log")
}

func TestGenerateSystemdUnit(t *testing.T) {
	binary := "/usr/local/bin/sofia"

	unit := generateSystemdUnit(binary)

	assert.Contains(t, unit, "ExecStart=/usr/local/bin/sofia gateway")
	assert.Contains(t, unit, "Restart=always")
	assert.Contains(t, unit, "RestartSec=5")
	assert.Contains(t, unit, "Description=Sofia AI Gateway")
	assert.Contains(t, unit, "WantedBy=default.target")
}

func TestNewDaemonCommand(t *testing.T) {
	cmd := NewDaemonCommand()

	require.NotNil(t, cmd)
	assert.Equal(t, "daemon", cmd.Use)
	assert.Equal(t, "Manage Sofia as a background service", cmd.Short)

	require.True(t, cmd.HasSubCommands())

	subcommands := cmd.Commands()
	require.Len(t, subcommands, 3)

	names := make([]string, 0, len(subcommands))
	for _, sub := range subcommands {
		names = append(names, sub.Name())
	}

	assert.Contains(t, names, "install")
	assert.Contains(t, names, "uninstall")
	assert.Contains(t, names, "status")
}
