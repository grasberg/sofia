// Package auth implements the `sofia auth` subcommand tree: browser-based
// OAuth login, credential removal, and status inspection for every provider
// whose flow is defined in pkg/auth (OpenAI/ChatGPT, Qwen, Google
// Antigravity). All long-lived state lives in ~/.sofia/auth.json via
// pkg/auth.{Get,Set,Delete}Credential so other subsystems — the Codex, Qwen,
// and Antigravity provider factories — can pick up the tokens without any
// extra plumbing.
package auth

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	pkgauth "github.com/grasberg/sofia/pkg/auth"
)

// flow describes one OAuth provider the CLI knows how to log into. The
// lookup table below keeps the CLI decoupled from pkg/auth internals — the
// "provider" key in the store is just the short name, and the config
// closure pins the right endpoint/client-id combination per vendor.
type flow struct {
	key   string                         // credential-store key and CLI alias
	label string                         // human name for help text and status output
	login func() (*pkgauth.AuthCredential, error)
}

func flows() []flow {
	return []flow{
		{
			key:   "openai",
			label: "OpenAI (ChatGPT OAuth)",
			login: func() (*pkgauth.AuthCredential, error) {
				return pkgauth.LoginBrowser(pkgauth.OpenAIOAuthConfig())
			},
		},
		{
			key:   "qwen",
			label: "Qwen (chat.qwen.ai)",
			login: func() (*pkgauth.AuthCredential, error) {
				return pkgauth.LoginQwenBrowser(pkgauth.QwenOAuthConfig())
			},
		},
		{
			key:   "google-antigravity",
			label: "Google Antigravity",
			login: func() (*pkgauth.AuthCredential, error) {
				return pkgauth.LoginBrowser(pkgauth.GoogleAntigravityOAuthConfig())
			},
		},
	}
}

// NewAuthCommand returns the `sofia auth` subcommand tree.
func NewAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Sign in to providers that use OAuth (OpenAI/ChatGPT, Qwen, Google)",
		Long: "Manage OAuth credentials stored at ~/.sofia/auth.json. " +
			"Other subsystems (the Codex, Qwen and Antigravity provider factories) " +
			"auto-refresh and consume these tokens — no other wiring is needed " +
			"once `auth login` succeeds.",
		Args: cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return c.Help()
		},
	}
	cmd.AddCommand(newLoginCommand(), newLogoutCommand(), newStatusCommand())
	return cmd
}

func newLoginCommand() *cobra.Command {
	var provider string
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to an OAuth provider via browser",
		Long: "Opens your default browser to the provider's OAuth authorization " +
			"page, runs a short-lived localhost callback, and stores the returned " +
			"tokens (plus refresh token) in ~/.sofia/auth.json.\n\n" +
			"Supported providers: " + strings.Join(providerKeys(), ", "),
		Example: "  sofia auth login --provider openai\n" +
			"  sofia auth login --provider qwen",
		Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runLogin(provider)
		},
	}
	cmd.Flags().StringVarP(&provider, "provider", "p", "",
		"OAuth provider to log in to ("+strings.Join(providerKeys(), "|")+")")
	_ = cmd.MarkFlagRequired("provider")
	return cmd
}

func newLogoutCommand() *cobra.Command {
	var provider string
	var all bool
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Remove stored OAuth credentials",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			if all {
				if err := pkgauth.DeleteAllCredentials(); err != nil {
					return fmt.Errorf("deleting all credentials: %w", err)
				}
				fmt.Println("✓ Removed all stored OAuth credentials.")
				return nil
			}
			if provider == "" {
				return fmt.Errorf("--provider is required (or pass --all)")
			}
			if err := pkgauth.DeleteCredential(provider); err != nil {
				return fmt.Errorf("deleting credential for %s: %w", provider, err)
			}
			fmt.Printf("✓ Removed stored credential for %s.\n", provider)
			return nil
		},
	}
	cmd.Flags().StringVarP(&provider, "provider", "p", "", "OAuth provider key to log out of")
	cmd.Flags().BoolVar(&all, "all", false, "Remove every stored credential")
	return cmd
}

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show which providers have stored OAuth credentials",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runStatus()
		},
	}
}

// runLogin drives one OAuth round-trip for the provider named on the CLI.
// Validation of the name is deliberately strict — passing an unknown one
// silently saved under the wrong key would break provider factories later.
func runLogin(providerKey string) error {
	f, ok := findFlow(providerKey)
	if !ok {
		return fmt.Errorf("unknown provider %q; supported: %s",
			providerKey, strings.Join(providerKeys(), ", "))
	}

	fmt.Printf("Starting %s login…\n", f.label)
	cred, err := f.login()
	if err != nil {
		return fmt.Errorf("%s login: %w", f.label, err)
	}
	if cred == nil {
		return fmt.Errorf("%s login returned no credential", f.label)
	}
	cred.Provider = f.key
	if cred.AuthMethod == "" {
		cred.AuthMethod = "oauth"
	}
	if err := pkgauth.SetCredential(f.key, cred); err != nil {
		return fmt.Errorf("saving credential: %w", err)
	}
	fmt.Printf("\n✓ Logged in to %s.\n", f.label)
	if cred.AccountID != "" {
		fmt.Printf("  Account: %s\n", cred.AccountID)
	}
	if !cred.ExpiresAt.IsZero() {
		fmt.Printf("  Access token expires: %s\n", cred.ExpiresAt.Local().Format(time.RFC3339))
	}
	fmt.Println("\nIn Settings → AI Models, enable a model whose Auth Method is OAuth.")
	return nil
}

// runStatus prints a one-line summary per flow (logged-in or not) plus the
// access-token expiry so users can tell at a glance which credentials are
// about to auto-refresh vs. will need a fresh login.
func runStatus() error {
	store, err := pkgauth.LoadStore()
	if err != nil {
		return fmt.Errorf("loading auth store: %w", err)
	}
	fmt.Println("Provider             Status     Expires")
	fmt.Println("-----------------    --------   -------")
	for _, f := range flows() {
		cred := store.Credentials[f.key]
		status := "—"
		expires := ""
		if cred != nil && cred.AccessToken != "" {
			if cred.IsExpired() {
				status = "expired"
			} else {
				status = "logged in"
			}
			if !cred.ExpiresAt.IsZero() {
				expires = cred.ExpiresAt.Local().Format("2006-01-02 15:04")
			} else {
				expires = "never"
			}
		}
		fmt.Printf("%-20s %-10s %s\n", f.key, status, expires)
	}
	return nil
}

func findFlow(key string) (flow, bool) {
	for _, f := range flows() {
		if f.key == key {
			return f, true
		}
	}
	return flow{}, false
}

func providerKeys() []string {
	keys := make([]string, 0, len(flows()))
	for _, f := range flows() {
		keys = append(keys, f.key)
	}
	sort.Strings(keys)
	return keys
}
