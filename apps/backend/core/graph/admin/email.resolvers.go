package admin

import (
	"context"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	signalsv1 "github.com/skeletg/skelet/apps/backend/core/signalsapi/v1"
)

func (r *mutationResolver) SaveEmailBlock(ctx context.Context, input model.SaveEmailBlockInput) (*model.EmailBlockSnippet, error) {
	authed, err := r.signalsChannel(ctx, "emails.manage", input.ChannelID)
	if err != nil {
		return nil, err
	}
	blocks, err := jsonText(input.Blocks)
	if err != nil {
		return nil, errInvalidInput("blocks", err)
	}
	fields, err := jsonText(input.Fields)
	if err != nil {
		return nil, errInvalidInput("fields", err)
	}
	res, err := r.SignalsAPI.Builder.SaveEmailBlock(authed, &signalsv1.SaveEmailBlockRequest{Block: &signalsv1.EmailBlock{
		ChannelId: input.ChannelID, Slug: input.Slug, Name: input.Name, Icon: optionalStr(input.Icon),
		FieldsJson: fields, BlocksJson: blocks, HtmlTemplate: optionalStr(input.HTMLTemplate), TextTemplate: optionalStr(input.TextTemplate),
		Css: optionalStr(input.CSS), Scripted: input.Scripted != nil && *input.Scripted,
	}})
	if err != nil {
		return nil, signalsErr(err)
	}
	return emailBlockModel(res.GetBlock()), nil
}

func (r *mutationResolver) DeleteEmailBlock(ctx context.Context, channelID string, slug string) (bool, error) {
	authed, err := r.signalsChannel(ctx, "emails.manage", channelID)
	if err != nil {
		return false, err
	}
	if _, err := r.SignalsAPI.Builder.DeleteEmailBlock(authed, &signalsv1.DeleteEmailBlockRequest{ChannelId: channelID, Slug: slug}); err != nil {
		return false, signalsErr(err)
	}
	return true, nil
}

func (r *queryResolver) EmailBlocks(ctx context.Context, channelID string) ([]*model.EmailBlockSnippet, error) {
	authed, err := r.signalsChannel(ctx, "emails.view", channelID)
	if err != nil {
		return nil, err
	}
	res, err := r.SignalsAPI.Builder.ListEmailBlocks(authed, &signalsv1.ListEmailBlocksRequest{ChannelId: channelID})
	if err != nil {
		return nil, signalsErr(err)
	}
	out := make([]*model.EmailBlockSnippet, 0, len(res.GetBlocks()))
	for _, b := range res.GetBlocks() {
		out = append(out, emailBlockModel(b))
	}
	return out, nil
}
