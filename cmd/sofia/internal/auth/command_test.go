package auth

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	pkgauth "github.com/grasberg/sofia/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// isolateAuthStore redirects ~/.sofia/auth.json to a temp dir so tests can
// mutate credentials without touching the user's real store.
func isolateAuthStore(t *testing.T) {
	t.Helper()
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".sofia"), 0o700))
	t.Setenv("HOME", tmp)
}

// captureStdout swaps os.Stdout for a pipe, runs f, and returns everything
// that was written. The auth command's `status` output goes to stdout via
// fmt.Println, so that's what we need to inspect.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = w

	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	f()

	_ = w.Close()
	os.Stdout = orig
	return <-done
}

func TestNewAuthCommand(t *testing.T) {
	cmd := NewAuthCommand()
	require.NotNil(t, cmd)
	assert.Equal(t, "auth", cmd.Use)

	// login, logout, status
	sub := cmd.Commands()
	require.Len(t, sub, 3)
	names := []string{sub[0].Name(), sub[1].Name(), sub[2].Name()}
	assert.ElementsMatch(t, []string{"login", "logout", "status"}, names)

	// login and logout both take --provider
	login, _, _ := cmd.Find([]string{"login"})
	require.NotNil(t, login)
	assert.NotNil(t, login.Flag("provider"), "login should expose --provider")

	logout, _, _ := cmd.Find([]string{"logout"})
	require.NotNil(t, logout)
	assert.NotNil(t, logout.Flag("provider"), "logout should expose --provider")
	assert.NotNil(t, logout.Flag("all"), "logout should expose --all")
}

// TestRunStatus_EmptyStoreListsAllProviders ensures the status output lists
// every supported flow even when nothing has been logged in yet, so users
// can discover available OAuth providers without reading docs.
func TestRunStatus_EmptyStoreListsAllProviders(t *testing.T) {
	isolateAuthStore(t)

	out := captureStdout(t, func() {
		require.NoError(t, runStatus())
	})

	for _, key := range providerKeys() {
		assert.Contains(t, out, key, "status should mention provider %q", key)
	}
	assert.Contains(t, out, "—", "empty rows should show a dash placeholder, not a status")
}

// TestRunStatus_ReflectsStoredCredential covers the happy path: after a
// credential is written to the store the status command surfaces "logged in"
// for that provider with the correct expiry.
func TestRunStatus_ReflectsStoredCredential(t *testing.T) {
	isolateAuthStore(t)

	expiry := time.Now().Add(1 * time.Hour)
	require.NoError(t, pkgauth.SetCredential("openai", &pkgauth.AuthCredential{
		AccessToken: "tok",
		Provider:    "openai",
		AuthMethod:  "oauth",
		ExpiresAt:   expiry,
	}))

	out := captureStdout(t, func() {
		require.NoError(t, runStatus())
	})

	assert.Contains(t, out, "openai")
	assert.Contains(t, out, "logged in")
	assert.Contains(t, out, expiry.Local().Format("2006-01-02 15:04"))
}

// TestRunStatus_MarksExpiredCredential verifies the status output flips to
// "expired" once the access token's expiry is in the past so users can tell
// why a request might have failed even though a credential exists.
func TestRunStatus_MarksExpiredCredential(t *testing.T) {
	isolateAuthStore(t)

	require.NoError(t, pkgauth.SetCredential("qwen", &pkgauth.AuthCredential{
		AccessToken: "old",
		Provider:    "qwen",
		AuthMethod:  "oauth",
		ExpiresAt:   time.Now().Add(-1 * time.Hour),
	}))

	out := captureStdout(t, func() {
		require.NoError(t, runStatus())
	})

	qwenLine := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "qwen") {
			qwenLine = line
			break
		}
	}
	require.NotEmpty(t, qwenLine, "qwen row should be present")
	assert.Contains(t, qwenLine, "expired")
}

// TestRunLogin_RejectsUnknownProvider guards the CLI surface: typo'd names
// must not silently save under the wrong key (which would later fail with a
// confusing "no credentials" error from the provider factory).
func TestRunLogin_RejectsUnknownProvider(t *testing.T) {
	err := runLogin("does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown provider")
	for _, key := range providerKeys() {
		assert.Contains(t, err.Error(), key, "error should list valid provider %q", key)
	}
}

// TestLogoutCommand_RemovesStoredCredential exercises the logout path via
// the cobra command tree (catches flag wiring regressions in addition to
// the deletion logic).
func TestLogoutCommand_RemovesStoredCredential(t *testing.T) {
	isolateAuthStore(t)

	require.NoError(t, pkgauth.SetCredential("openai", &pkgauth.AuthCredential{
		AccessToken: "tok", Provider: "openai", AuthMethod: "oauth",
	}))

	cmd := NewAuthCommand()
	cmd.SetArgs([]string{"logout", "--provider", "openai"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	// The fmt.Printf calls inside runLogout write to os.Stdout directly;
	// silence them so `go test -v` output stays clean.
	_ = captureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	cred, err := pkgauth.GetCredential("openai")
	require.NoError(t, err)
	assert.Nil(t, cred, "credential should be gone after logout")
}

// TestLogoutCommand_AllClearsEverything covers the --all escape hatch used
// when a user wants to reset every stored OAuth credential at once.
func TestLogoutCommand_AllClearsEverything(t *testing.T) {
	isolateAuthStore(t)

	require.NoError(t, pkgauth.SetCredential("openai", &pkgauth.AuthCredential{AccessToken: "a", Provider: "openai"}))
	require.NoError(t, pkgauth.SetCredential("qwen", &pkgauth.AuthCredential{AccessToken: "b", Provider: "qwen"}))

	cmd := NewAuthCommand()
	cmd.SetArgs([]string{"logout", "--all"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	_ = captureStdout(t, func() {
		require.NoError(t, cmd.Execute())
	})

	for _, key := range []string{"openai", "qwen"} {
		cred, err := pkgauth.GetCredential(key)
		require.NoError(t, err)
		assert.Nil(t, cred, "credential %q should be gone after --all", key)
	}
}

// TestLogoutCommand_RequiresProviderOrAll ensures we fail fast when neither
// is provided, rather than silently doing nothing.
func TestLogoutCommand_RequiresProviderOrAll(t *testing.T) {
	isolateAuthStore(t)

	cmd := NewAuthCommand()
	cmd.SetArgs([]string{"logout"})
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	_ = captureStdout(t, func() {
		err := cmd.Execute()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "--provider is required")
	})
}
