package web

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"

	"github.com/grasberg/sofia/pkg/config"
)

// sensitiveJSONFields lists the JSON field names whose values must be masked
// before returning configuration data to the client.
var sensitiveJSONFields = map[string]bool{
	"api_key":        true,
	"token":          true,
	"password":       true,
	"passphrase":     true,
	"secret_api_key": true,
	"secret":         true,
	"auth_token":     true,
	"api_token":      true,
}

// maskSecrets takes a JSON-serialisable value as a map/slice/etc. and
// replaces the values of sensitive fields with "********".
func maskSecrets(v any) any {
	switch val := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, child := range val {
			if sensitiveJSONFields[k] {
				if s, ok := child.(string); ok && s != "" {
					out[k] = "********"
					continue
				}
			}
			out[k] = maskSecrets(child)
		}
		return out
	case []any:
		out := make([]any, len(val))
		for i, child := range val {
			out[i] = maskSecrets(child)
		}
		return out
	default:
		return v
	}
}

// configToMaskedJSON marshals a config to JSON, decodes into a generic
// map, masks secrets, then re-encodes. This avoids modifying the original.
func configToMaskedJSON(cfg *config.Config) ([]byte, error) {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil, err
	}
	masked := maskSecrets(generic)
	return json.Marshal(masked)
}

// maskedPlaceholder is the value used to replace sensitive fields in API responses.
const maskedPlaceholder = "********"

// restoreMaskedSecrets walks two generic JSON trees (incoming and original)
// in lockstep. Wherever a sensitive field in incoming still contains the
// maskedPlaceholder, it is replaced with the real value from original.
// This prevents the Web UI "save config" flow from overwriting real API
// keys with the masked placeholder.
func restoreMaskedSecrets(incoming, original any) any {
	switch inc := incoming.(type) {
	case map[string]any:
		orig, _ := original.(map[string]any)
		for k, child := range inc {
			if sensitiveJSONFields[k] {
				if s, ok := child.(string); ok && s == maskedPlaceholder {
					if orig != nil {
						if origVal, exists := orig[k]; exists {
							inc[k] = origVal
							continue
						}
					}
				}
			}
			var origChild any
			if orig != nil {
				origChild = orig[k]
			}
			inc[k] = restoreMaskedSecrets(child, origChild)
		}
		return inc
	case []any:
		origSlice, _ := original.([]any)
		for i, child := range inc {
			var origChild any
			if i < len(origSlice) {
				origChild = origSlice[i]
			}
			inc[i] = restoreMaskedSecrets(child, origChild)
		}
		return inc
	default:
		return incoming
	}
}

func resolveAssetsDir() string {
	if custom := os.Getenv("SOFIA_ASSETS_DIR"); custom != "" {
		if stat, err := os.Stat(custom); err == nil && stat.IsDir() {
			return custom
		}
	}

	wd, _ := os.Getwd()
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)

	candidates := []string{
		filepath.Join(wd, "assets"),
		filepath.Join(exeDir, "assets"),
		filepath.Join(exeDir, "..", "assets"),
		filepath.Join(exeDir, "..", "share", "sofia", "assets"),
	}

	for _, dir := range candidates {
		if stat, err := os.Stat(dir); err == nil && stat.IsDir() {
			return dir
		}
	}

	return "assets"
}

// mustJSON marshals v to a JSON string, returning "{}" on error.
func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func (s *Server) sendJSONError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
