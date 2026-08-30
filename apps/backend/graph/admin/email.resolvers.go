package admin

import (
	"context"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/emailtemplate"
)

func (r *mutationResolver) SaveEmailBlock(ctx context.Context, input model.SaveEmailBlockInput) (*model.EmailBlockSnippet, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(input.ChannelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	blocks, err := blocksFromJSON(input.Blocks)
	if err != nil {
		return nil, err
	}
	fields, err := fieldsFromJSON(input.Fields)
	if err != nil {
		return nil, err
	}
	sn := emailtemplate.Snippet{ChannelID: cid, Slug: input.Slug, Name: input.Name, Blocks: blocks, Fields: fields}
	if input.Icon != nil {
		sn.Icon = *input.Icon
	}
	if input.HTMLTemplate != nil {
		sn.HTMLTemplate = *input.HTMLTemplate
	}
	if input.TextTemplate != nil {
		sn.TextTemplate = *input.TextTemplate
	}
	if input.CSS != nil {
		sn.CSS = *input.CSS
	}
	if input.Scripted != nil {
		sn.Scripted = *input.Scripted
	}
	if err := r.EmailTmplSvc.SaveBlock(ctx, sn); err != nil {
		return nil, err
	}
	return toEmailBlockModel(sn), nil
}

func (r *mutationResolver) DeleteEmailBlock(ctx context.Context, channelID string, slug string) (bool, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.manage"); err != nil {
		return false, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return false, errInvalidID("channelId", err)
	}
	if err := r.EmailTmplSvc.DeleteBlock(ctx, cid, slug); err != nil {
		return false, err
	}
	return true, nil
}

func (r *queryResolver) EmailBlocks(ctx context.Context, channelID string) ([]*model.EmailBlockSnippet, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "emails.view"); err != nil {
		return nil, err
	}
	cid, err := uuid.Parse(channelID)
	if err != nil {
		return nil, errInvalidID("channelId", err)
	}
	snips, err := r.EmailTmplSvc.ListBlocks(ctx, cid)
	if err != nil {
		return nil, err
	}
	out := make([]*model.EmailBlockSnippet, len(snips))
	for i, sn := range snips {
		out[i] = toEmailBlockModel(sn)
	}
	return out, nil
}
