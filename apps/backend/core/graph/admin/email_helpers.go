package admin

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

// A reusable block as the dashboard shows it - the field schema and the body stay JSON documents
func emailBlockModel(b *signalsv1.EmailBlock) *model.EmailBlockSnippet {
	return &model.EmailBlockSnippet{
		ChannelID:    b.GetChannelId(),
		Slug:         b.GetSlug(),
		Name:         b.GetName(),
		Icon:         b.GetIcon(),
		Fields:       jsonValue(b.GetFieldsJson(), "[]"),
		Blocks:       jsonValue(b.GetBlocksJson(), "[]"),
		HTMLTemplate: b.GetHtmlTemplate(),
		TextTemplate: b.GetTextTemplate(),
		CSS:          b.GetCss(),
		Scripted:     b.GetScripted(),
	}
}
