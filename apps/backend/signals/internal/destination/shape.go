package destination

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Payload shaping per destination kind - pure functions, unit-testable without credentials or network

// The Measurement Protocol's per-request event ceiling - it answers 2xx even to a payload it discards, so the cap is ours to keep
const ga4MaxEvents = 25

func ShapePayload(kind string, batch []Event) ([][]byte, error) {
	switch kind {
	case KindGA4:
		return shapeGA4(batch)
	case KindMetaCAPI:
		return one(shapeMetaCAPI(batch))
	default:
		return one(json.Marshal(map[string]any{"events": batch, "count": len(batch)}))
	}
}

func one(body []byte, err error) ([][]byte, error) {
	if err != nil {
		return nil, err
	}
	return [][]byte{body}, nil
}

// shapeGA4 renders GA4 Measurement Protocol - one {client_id, events} object per request, so the batch is grouped by visitor
// identity (anon id, else customer id) and each group split at the event ceiling - Event names are snake_cased for MP rules
func shapeGA4(batch []Event) ([][]byte, error) {
	type mpEvent struct {
		Name   string         `json:"name"`
		Params map[string]any `json:"params,omitempty"`
	}
	type mpEnvelope struct {
		ClientID string    `json:"client_id"`
		Events   []mpEvent `json:"events"`
	}
	byClient := map[string]*mpEnvelope{}
	var order []string
	for _, e := range batch {
		cid := e.AnonID
		if cid == "" && e.CustomerID != nil {
			cid = e.CustomerID.String()
		}
		if cid == "" {
			cid = "anonymous"
		}
		env, ok := byClient[cid]
		if !ok {
			env = &mpEnvelope{ClientID: cid}
			byClient[cid] = env
			order = append(order, cid)
		}
		var params map[string]any
		if len(e.Properties) > 0 {
			_ = json.Unmarshal(e.Properties, &params)
		}
		env.Events = append(env.Events, mpEvent{Name: ga4Name(e.Name), Params: params})
	}
	var out [][]byte
	for _, cid := range order {
		events := byClient[cid].Events
		for start := 0; start < len(events); start += ga4MaxEvents {
			body, err := json.Marshal(mpEnvelope{ClientID: cid, Events: events[start:min(start+ga4MaxEvents, len(events))]})
			if err != nil {
				return nil, err
			}
			out = append(out, body)
		}
	}
	return out, nil
}

func ga4Name(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.NewReplacer(" ", "_", "-", "_", ".", "_", ":", "_").Replace(s)
	if s == "" {
		return "custom_event"
	}
	return s
}

// shapeMetaCAPI renders the Meta Conversions API envelope - Matching rides hashed
// external_id (customer uuid) - email hashes come from audience sync, not events
func shapeMetaCAPI(batch []Event) ([]byte, error) {
	type capiEvent struct {
		EventName    string         `json:"event_name"`
		EventTime    int64          `json:"event_time"`
		EventID      string         `json:"event_id"`
		ActionSource string         `json:"action_source"`
		UserData     map[string]any `json:"user_data"`
		CustomData   map[string]any `json:"custom_data,omitempty"`
	}
	data := make([]capiEvent, 0, len(batch))
	for _, e := range batch {
		ud := map[string]any{}
		if e.CustomerID != nil {
			ud["external_id"] = []string{HashIdentifier(e.CustomerID.String())}
		} else if e.AnonID != "" {
			ud["external_id"] = []string{HashIdentifier(e.AnonID)}
		}
		var custom map[string]any
		if len(e.Properties) > 0 {
			_ = json.Unmarshal(e.Properties, &custom)
		}
		data = append(data, capiEvent{
			EventName:    e.Name,
			EventTime:    e.OccurredAt.Unix(),
			EventID:      e.EventID, // Meta dedupes on (event_name, event_id)
			ActionSource: capiActionSource(e.Source),
			UserData:     ud,
			CustomData:   custom,
		})
	}
	return json.Marshal(map[string]any{"data": data})
}

// capiActionSource maps a spine source to a valid Meta CAPI action_source - web = on-site, campaign/flow/transactional
// = email, else server-generated (Collapsing everything non-web to "email" mislabeled server commerce events.)
func capiActionSource(source string) string {
	switch source {
	case "web":
		return "website"
	case "campaign", "flow", "transactional":
		return "email"
	default:
		return "system_generated"
	}
}

// HashIdentifier is the ad-platform normalization - trim, lowercase, SHA-256 hex,
// the recipe Meta and Google Customer Match specify for emails
func HashIdentifier(v string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(v))))
	return hex.EncodeToString(sum[:])
}

// BuildAudiencePayload renders the JSON body SyncAudience posts - the identifiers arrive hashed
func BuildAudiencePayload(segmentName string, identifiers []string) ([]byte, []string) {
	hashes := make([]string, 0, len(identifiers))
	for _, h := range identifiers {
		if strings.TrimSpace(h) == "" {
			continue
		}
		hashes = append(hashes, h)
	}
	body, _ := json.Marshal(map[string]any{
		"audience":     segmentName,
		"schema":       "EMAIL_SHA256",
		"identifiers":  hashes,
		"member_count": len(hashes),
	})
	return body, hashes
}
