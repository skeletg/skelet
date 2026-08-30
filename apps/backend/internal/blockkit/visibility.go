package blockkit

import "time"

// Per-block visibility rules, stored free-form in a block's props under "visibility" with no migration, since Validate leaves props untouched
// The shape is {startAt, endAt} as RFC3339 strings bounding when the block shows, {segments} as an allowlist of slugs, and {devices} as a client-side hint
// Schedule and segment allowlists are evaluated SERVER-SIDE in FilterServerSide, so hidden content never ships and it stays SEO-safe - device rules ride on the shipped prop and apply client-side through CSS, keeping the page cacheable for anonymous visitors
type Visibility struct {
	Present  bool
	StartAt  *time.Time
	EndAt    *time.Time
	Segments []string
	Devices  []string
}

type EvalContext struct { // the per-request audience the server filters against
	Now      time.Time
	Segments map[string]bool // the viewer's segment slugs ("" set = anonymous)
}

func ParseVisibility(props map[string]any) Visibility { // Present is false when the block carries no rule
	var v Visibility
	m, ok := props["visibility"].(map[string]any)
	if !ok {
		return v
	}
	v.Present = true
	if s, ok := m["startAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			v.StartAt = &t
		}
	}
	if s, ok := m["endAt"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			v.EndAt = &t
		}
	}
	v.Segments = toStringSlice(m["segments"])
	v.Devices = toStringSlice(m["devices"])
	return v
}

func FilterServerSide(doc Document, ec EvalContext) Document { // drops blocks whose schedule window is closed or whose segment allowlist excludes the viewer, recursing into children - a kept block gets its audience rule STRIPPED from the shipped props so targeting isn't leaked to the client, only devices surviving for client-side CSS, and a block with no rule is untouched
	return Document{Version: doc.Version, Blocks: filterBlocks(doc.Blocks, ec)}
}

func filterBlocks(in []Block, ec EvalContext) []Block {
	out := make([]Block, 0, len(in))
	for _, b := range in {
		vis := ParseVisibility(b.Props)
		if vis.Present {
			if vis.StartAt != nil && ec.Now.Before(*vis.StartAt) {
				continue
			}
			if vis.EndAt != nil && ec.Now.After(*vis.EndAt) {
				continue
			}
			if len(vis.Segments) > 0 && !anyInSet(vis.Segments, ec.Segments) {
				continue // allowlist excludes this viewer (incl. anonymous)
			}
			b.Props = stripVisibility(b.Props, vis.Devices)
		}
		if len(b.Blocks) > 0 {
			b.Blocks = filterBlocks(b.Blocks, ec)
		}
		out = append(out, b)
	}
	return out
}

func anyInSet(want []string, have map[string]bool) bool {
	for _, w := range want {
		if have[w] {
			return true
		}
	}
	return false
}

func stripVisibility(props map[string]any, devices []string) map[string]any { // only the device hint is re-added, so audience and schedule info never reaches the client
	cp := make(map[string]any, len(props))
	for k, v := range props {
		if k != "visibility" {
			cp[k] = v
		}
	}
	if len(devices) > 0 {
		cp["visibility"] = map[string]any{"devices": devices}
	}
	return cp
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, x := range arr {
		if s, ok := x.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
