package admin

import (
	"encoding/json"
	"fmt"

	"github.com/skeletg/skelet/apps/backend/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/internal/emailtemplate"
)

// Lives here rather than a gqlgen-managed *.resolvers.go so regen doesn't quarantine these

// Per-block field validation happens in the service on save, not here
func blocksFromJSON(v any) ([]emailtemplate.Block, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encode blocks: %w", err)
	}
	var blocks []emailtemplate.Block
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return nil, fmt.Errorf("blocks must be a JSON array of {type,...}: %w", err)
	}
	return blocks, nil
}

func stylesFromJSON(v any) (emailtemplate.EmailStyles, error) { // nil/absent yields the zero theme
	var s emailtemplate.EmailStyles
	if v == nil {
		return s, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return s, fmt.Errorf("encode styles: %w", err)
	}
	if err := json.Unmarshal(raw, &s); err != nil {
		return s, fmt.Errorf("styles must be a JSON object: %w", err)
	}
	return s, nil
}

func toEmailBlockModel(sn emailtemplate.Snippet) *model.EmailBlockSnippet {
	fields := sn.Fields
	if fields == nil {
		fields = []emailtemplate.FieldDef{}
	}
	return &model.EmailBlockSnippet{
		ChannelID:    sn.ChannelID.String(),
		Slug:         sn.Slug,
		Name:         sn.Name,
		Icon:         sn.Icon,
		Fields:       fields,
		Blocks:       blocksOrEmpty(sn.Blocks),
		HTMLTemplate: sn.HTMLTemplate,
		TextTemplate: sn.TextTemplate,
		CSS:          sn.CSS,
		Scripted:     sn.Scripted,
	}
}

func fieldsFromJSON(v any) ([]emailtemplate.FieldDef, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var fs []emailtemplate.FieldDef
	if err := json.Unmarshal(raw, &fs); err != nil {
		return nil, fmt.Errorf("fields must be a JSON array of {key,label,kind,…}: %w", err)
	}
	return fs, nil
}

// Nil normalizes to empty so the JSON scalar serializes as `[]` rather than `null`
func blocksOrEmpty(b []emailtemplate.Block) []emailtemplate.Block {
	if b == nil {
		return []emailtemplate.Block{}
	}
	return b
}
