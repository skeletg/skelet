package admin

import (
	"encoding/json"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// A save's extensions as each plugin reads its part - an object, each plugin's part under its slug
func extensionsInput(v any) (map[string]json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, apperr.New("EXTENSIONS_INVALID", apperr.ClassValidation, "extensions must be an object, each plugin's part under its slug")
	}
	out := make(map[string]json.RawMessage, len(m))
	for k, part := range m {
		b, err := json.Marshal(part)
		if err != nil {
			return nil, apperr.New("EXTENSIONS_INVALID", apperr.ClassValidation, "the "+k+" extension doesn't read as JSON")
		}
		out[k] = b
	}
	return out, nil
}
