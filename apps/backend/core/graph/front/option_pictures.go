package front

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/front/dataloaders"
	model1 "github.com/skeletg/skelet/apps/backend/core/graph/front/model"
)

// Option and choice pictures, loaded in one batch and only when the query asks for them - the product page's main
// query doesn't, so it never pays for the lookup
type optionPictures struct {
	wantOption, wantValue bool
	pending               []pendingPicture
}

type pendingPicture struct {
	id  uuid.UUID
	set func(*model1.ProductCustomOptionImage)
}

func newOptionPictures(ctx context.Context) *optionPictures {
	p := &optionPictures{}
	fc := graphql.GetFieldContext(ctx)
	if fc == nil || !graphql.HasOperationContext(ctx) {
		return p
	}
	op := graphql.GetOperationContext(ctx)
	for _, f := range graphql.CollectFields(op, fc.Field.Selections, []string{"ProductCustomOption"}) {
		switch f.Name {
		case "image":
			p.wantOption = true
		case "values":
			for _, vf := range graphql.CollectFields(op, f.Selections, []string{"ProductCustomOptionValue"}) {
				p.wantValue = p.wantValue || vf.Name == "image"
			}
		}
	}
	return p
}

func (p *optionPictures) option(m *model1.ProductCustomOption, id *uuid.UUID) {
	if p.wantOption && id != nil {
		p.pending = append(p.pending, pendingPicture{*id, func(img *model1.ProductCustomOptionImage) { m.Image = img }})
	}
}

func (p *optionPictures) value(m *model1.ProductCustomOptionValue, id *uuid.UUID) {
	if p.wantValue && id != nil {
		p.pending = append(p.pending, pendingPicture{*id, func(img *model1.ProductCustomOptionImage) { m.Image = img }})
	}
}

// A picture that's gone or fails to load leaves the option without one rather than failing the product
func (p *optionPictures) load(ctx context.Context) {
	if len(p.pending) == 0 {
		return
	}
	loaders := dataloaders.For(ctx)
	if loaders == nil {
		return
	}
	ids := make([]uuid.UUID, len(p.pending))
	for i, pp := range p.pending {
		ids[i] = pp.id
	}
	assets, _ := loaders.MediaAsset.LoadMany(ctx, ids)()
	for i, pp := range p.pending {
		if i < len(assets) && assets[i].ID != uuid.Nil {
			a := assets[i]
			pp.set(&model1.ProductCustomOptionImage{URL: a.URL, AltText: a.AltText, Width: a.Width, Height: a.Height})
		}
	}
}
