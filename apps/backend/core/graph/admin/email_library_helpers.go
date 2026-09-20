package admin

import (
	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func libraryEmailModel(e *signalsv1.LibraryEmail) *model.LibraryEmail {
	return &model.LibraryEmail{
		ID:        e.GetId(),
		ChannelID: e.GetChannelId(),
		Name:      e.GetName(),
		Subject:   e.GetSubject(),
		Preheader: e.GetPreheader(),
		Blocks:    jsonValue(e.GetBlocksJson(), "[]"),
		Styles:    jsonValue(e.GetStylesJson(), "{}"),
		GroupID:   e.GroupId,
		Slot:      e.GetSlot(),
		CreatedAt: protoTime(e.GetCreatedAt()),
		UpdatedAt: protoTime(e.GetUpdatedAt()),
	}
}

func emailGroupModel(g *signalsv1.EmailGroup) *model.EmailGroup {
	return &model.EmailGroup{ID: g.GetId(), Slug: g.GetSlug(), Name: g.GetName(), IsSystem: g.GetIsSystem(), EmailCount: g.GetEmailCount()}
}
