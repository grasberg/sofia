package config

import (
	"strings"
	"testing"
)

// TestDefaultModelList_EveryEntryResolvesAPIBase guards the catalog-shortening
// invariant: every entry either sets APIBase explicitly or uses a Model
// protocol prefix covered by WellKnownProviderBases. If someone adds a new
// model with an unknown prefix and forgets the APIBase, the agent would error
// at call time with "no API base configured" — this test catches that at
// compile-adjacent time instead.
func TestDefaultModelList_EveryEntryResolvesAPIBase(t *testing.T) {
	for _, mc := range DefaultModelList() {
		mc := mc
		t.Run(mc.ModelName, func(t *testing.T) {
			if mc.ModelName == "" {
				t.Fatal("ModelName is empty")
			}
			if mc.Model == "" {
				t.Fatal("Model is empty")
			}
			if got := mc.ResolveAPIBase(); got == "" {
				t.Errorf("ResolveAPIBase() = %q; catalog entry must either set APIBase or "+
					"use a Model prefix listed in WellKnownProviderBases (got Model=%q, APIBase=%q)",
					got, mc.Model, mc.APIBase)
			}
		})
	}
}

// TestDefaultModelList_OpenAIOAuthEntries guards the contract that lets
// `sofia auth login --provider openai` users pick ChatGPT-backed models: at
// least one entry must use AuthMethod=oauth on an "openai/*" model prefix,
// and no such entry may set an API key (OAuth flows store tokens separately
// in ~/.sofia/auth.json). If a future edit drops AuthMethod here the
// factory would silently downgrade to api.openai.com which rejects ChatGPT
// OAuth access tokens.
func TestDefaultModelList_OpenAIOAuthEntries(t *testing.T) {
	var count int
	for _, mc := range DefaultModelList() {
		if mc.AuthMethod != "oauth" {
			continue
		}
		if !strings.HasPrefix(mc.Model, "openai/") {
			continue
		}
		count++
		if mc.APIKey != "" {
			t.Errorf("%s: OAuth entries must not ship with an API key", mc.ModelName)
		}
		if mc.ModelName == "" {
			t.Errorf("%s: ModelName is required", mc.DisplayName)
		}
	}
	if count == 0 {
		t.Fatal("expected at least one OpenAI OAuth catalog entry (AuthMethod=oauth + openai/ prefix)")
	}
}

// TestDefaultModelList_NoRedundantAPIBase flags catalog entries that hard-code
// an APIBase that ResolveAPIBase() would derive anyway. Redundancy isn't a
// bug, but the catalog is the DRY-free zone for endpoints — an explicit
// APIBase matching the well-known default silently shadows the well-known
// map's value, so a future correction to the map would leave the catalog
// stale. Keep APIBase only where it genuinely differs.
func TestDefaultModelList_NoRedundantAPIBase(t *testing.T) {
	for _, mc := range DefaultModelList() {
		if mc.APIBase == "" {
			continue
		}
		shadow := ModelConfig{Model: mc.Model}
		if resolved := shadow.ResolveAPIBase(); resolved != "" && resolved == mc.APIBase {
			t.Errorf("%s: APIBase=%q is identical to the WellKnownProviderBases default for "+
				"prefix %q — drop the field and let ResolveAPIBase derive it",
				mc.ModelName, mc.APIBase, mc.Model)
		}
	}
}
