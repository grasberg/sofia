package memory

import (
	"testing"

	"github.com/grasberg/sofia/pkg/config"
	"github.com/stretchr/testify/require"
)

// TestSeedCatalogModels_BackfillsMaxTokensField verifies the upgrade contract:
// when the catalog grows a new MaxTokensField recommendation for an existing
// model, a re-seed backfills rows that still have the empty default without
// overwriting values the user set manually (catalog rows are editable from
// the Settings UI via SyncModels).
func TestSeedCatalogModels_BackfillsMaxTokensField(t *testing.T) {
	db := openTestDB(t)

	// First run: seed without MaxTokensField so the backfill has something
	// to backfill on the second pass.
	initial := []config.ModelConfig{
		{ModelName: "o3", Provider: "OpenAI", DisplayName: "o3",
			Model: "openai/o3", APIBase: "https://api.openai.com/v1"},
		{ModelName: "gpt-4o", Provider: "OpenAI", DisplayName: "GPT-4o",
			Model: "openai/gpt-4o", APIBase: "https://api.openai.com/v1"},
		{ModelName: "custom-o3", Provider: "OpenAI", DisplayName: "Custom o3",
			Model: "openai/o3", APIBase: "https://api.openai.com/v1"},
	}
	require.NoError(t, db.SeedCatalogModels(initial))

	// Simulate a user override on "custom-o3" (they picked a non-empty value
	// via the Settings UI — SyncModels writes this column).
	_, err := db.db.Exec(`UPDATE models SET max_tokens_field = 'max_tokens' WHERE model_name = 'custom-o3'`)
	require.NoError(t, err)

	// Second run: new catalog recommends max_completion_tokens for both o3
	// entries. Backfill should only touch "o3" (empty default); "custom-o3"
	// stays as the user set it; "gpt-4o" stays empty (catalog doesn't set it).
	upgraded := []config.ModelConfig{
		{ModelName: "o3", Provider: "OpenAI", DisplayName: "o3",
			Model: "openai/o3", APIBase: "https://api.openai.com/v1",
			MaxTokensField: "max_completion_tokens"},
		{ModelName: "gpt-4o", Provider: "OpenAI", DisplayName: "GPT-4o",
			Model: "openai/gpt-4o", APIBase: "https://api.openai.com/v1"},
		{ModelName: "custom-o3", Provider: "OpenAI", DisplayName: "Custom o3",
			Model: "openai/o3", APIBase: "https://api.openai.com/v1",
			MaxTokensField: "max_completion_tokens"},
	}
	require.NoError(t, db.SeedCatalogModels(upgraded))

	got := map[string]string{}
	rows, err := db.db.Query(`SELECT model_name, max_tokens_field FROM models`)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var name, field string
		require.NoError(t, rows.Scan(&name, &field))
		got[name] = field
	}
	require.NoError(t, rows.Err())

	require.Equal(t, "max_completion_tokens", got["o3"], "o3 should be backfilled from empty to catalog value")
	require.Equal(t, "max_tokens", got["custom-o3"], "user override must not be overwritten by backfill")
	require.Equal(t, "", got["gpt-4o"], "catalog with no recommendation must leave the field empty")
}

// TestListConfiguredModels_OAuthCatalogRequiresOptIn pins the fix for the
// "AI Models page listed ~10 OAuth entries I never configured" bug: catalog
// rows with auth_method baked in (OpenAI ChatGPT OAuth, Qwen OAuth Free)
// must stay invisible until the user explicitly saves them via SyncModels.
//
// Before, ListConfiguredModels treated auth_method != '' as "configured",
// so seeding alone was enough to surface these rows.  Now enablement is
// tracked separately via user_configured, flipped by SyncModels and nothing
// else.
func TestListConfiguredModels_OAuthCatalogRequiresOptIn(t *testing.T) {
	db := openTestDB(t)

	require.NoError(t, db.SeedCatalogModels([]config.ModelConfig{
		{
			ModelName: "gpt-5.2-oauth", Provider: "OpenAI (ChatGPT)",
			DisplayName: "GPT-5.2 OAuth", Model: "openai/gpt-5.2",
			AuthMethod: "oauth",
		},
		{
			ModelName: "gpt-4o", Provider: "OpenAI",
			DisplayName: "GPT-4o", Model: "openai/gpt-4o",
		},
	}))

	// Right after seeding, nothing is configured — seeding alone never
	// opts the user in, regardless of auth_method.
	configured, err := db.ListConfiguredModels()
	require.NoError(t, err)
	require.Empty(t, configured,
		"seeded catalog entries must not appear as configured until the user opts in")

	// Simulate the user saving the OAuth entry from the Settings UI.
	// SyncModels echoes back the same catalog entry; api_key stays empty
	// because OAuth tokens live in ~/.sofia/auth.json.
	require.NoError(t, db.SyncModels([]config.ModelConfig{{
		ModelName: "gpt-5.2-oauth", Provider: "OpenAI (ChatGPT)",
		DisplayName: "GPT-5.2 OAuth", Model: "openai/gpt-5.2",
		AuthMethod: "oauth",
	}}, nil))

	configured, err = db.ListConfiguredModels()
	require.NoError(t, err)
	names := make(map[string]bool)
	for _, mc := range configured {
		names[mc.ModelName] = true
	}
	require.True(t, names["gpt-5.2-oauth"],
		"OAuth entry must become configured after the user saves it")
	require.False(t, names["gpt-4o"],
		"untouched catalog entries must remain unconfigured")
}

// TestSeedCatalogModels_PreservesAPIKey verifies the pre-existing contract
// that a user-set api_key survives a re-seed with updated catalog metadata.
func TestSeedCatalogModels_PreservesAPIKey(t *testing.T) {
	db := openTestDB(t)

	seed := []config.ModelConfig{{
		ModelName: "gpt-4o", Provider: "OpenAI", DisplayName: "GPT-4o",
		Model: "openai/gpt-4o", APIBase: "https://api.openai.com/v1",
	}}
	require.NoError(t, db.SeedCatalogModels(seed))

	_, err := db.db.Exec(`UPDATE models SET api_key = 'sk-user-key' WHERE model_name = 'gpt-4o'`)
	require.NoError(t, err)

	// Re-seed with an updated display name — the refresh path must rewrite
	// the display name but leave api_key alone.
	updated := []config.ModelConfig{{
		ModelName: "gpt-4o", Provider: "OpenAI", DisplayName: "GPT-4o (Latest)",
		Model: "openai/gpt-4o", APIBase: "https://api.openai.com/v1",
	}}
	require.NoError(t, db.SeedCatalogModels(updated))

	var apiKey, displayName string
	err = db.db.QueryRow(
		`SELECT api_key, display_name FROM models WHERE model_name = 'gpt-4o'`,
	).Scan(&apiKey, &displayName)
	require.NoError(t, err)
	require.Equal(t, "sk-user-key", apiKey)
	require.Equal(t, "GPT-4o (Latest)", displayName)
}
