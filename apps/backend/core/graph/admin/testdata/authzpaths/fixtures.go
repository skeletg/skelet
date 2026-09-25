package authzpaths

func (r *queryResolver) LeakCmsPageConditionalAdmin(ctx context.Context, slug *string, id *string, urlPath *string) (*model.CMSPage, error) {
	loc := r.activeLocale(ctx)
	isAdmin := auth.RequirePermission(ctx, r.AdminSvc, "cms.view") == nil
	gate := func(p cms.Page) (*model.CMSPage, error) {
		if !isAdmin && !p.IsPublished {
			return nil, nil
		}
		return toCMSPageModel(p), nil
	}
	switch {
	case id != nil && *id != "":
		uid, err := uuid.Parse(*id)
		if err != nil {
			return nil, errInvalidID("id", err)
		}
		p, err := r.CMSSvc.GetPage(ctx, uid, loc)
		if err != nil {
			return nil, err
		}
		return gate(p)
	case slug != nil && *slug != "":
		p, err := r.CMSSvc.GetPageBySlug(ctx, *slug, loc)
		if err != nil {
			return nil, err
		}
		return gate(p)
	}
	return nil, fmt.Errorf("slug, id, or urlPath is required")
}

func (r *queryResolver) LeakDiscardedGate(ctx context.Context) (*model.Thing, error) {
	_ = auth.RequirePermission(ctx, r.AdminSvc, "x.view")
	return r.svc.Thing(ctx)
}

func (r *queryResolver) LeakBareGateStatement(ctx context.Context) (*model.Thing, error) {
	auth.RequirePermission(ctx, r.AdminSvc, "x.view")
	return r.svc.Thing(ctx)
}

func (r *queryResolver) LeakUncheckedError(ctx context.Context) (*model.Thing, error) {
	err := auth.RequirePermission(ctx, r.AdminSvc, "x.view")
	log.Print(err)
	return r.svc.Thing(ctx)
}

func (r *queryResolver) LeakCheckWithoutReturn(ctx context.Context) (*model.Thing, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "x.view"); err != nil {
		log.Print(err)
	}
	return r.svc.Thing(ctx)
}

func (r *queryResolver) LeakAuthOnly(ctx context.Context) (*model.Thing, error) {
	if _, err := auth.RequireAuth(ctx); err != nil {
		return nil, err
	}
	return r.svc.Thing(ctx)
}

func (r *queryResolver) LeakDataWithError(ctx context.Context, id string) (*model.Thing, error) {
	p, err := r.svc.Get(ctx, id)
	return p, err
}

func (r *queryResolver) LeakInsideRefusal(ctx context.Context) (*model.Thing, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "x.view"); err != nil {
		return r.svc.Public(ctx), nil
	}
	return r.svc.Thing(ctx)
}

func (r *queryResolver) LeakGatePassesOnlyInBranch(ctx context.Context) (*model.Thing, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "x.view"); err == nil {
		log.Print("admin")
	}
	return r.svc.Thing(ctx)
}

func (r *queryResolver) LeakHelperAllowsEarly(ctx context.Context) (*model.Thing, error) {
	if err := r.leakyHelper(ctx); err != nil {
		return nil, err
	}
	return r.svc.Thing(ctx)
}

func (r *Resolver) leakyHelper(ctx context.Context) error {
	if r.DevMode {
		return nil
	}
	return auth.RequirePermission(ctx, r.AdminSvc, "x.view")
}

func (r *queryResolver) GatedInit(ctx context.Context) (*model.Thing, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "x.view"); err != nil {
		return nil, err
	}
	return r.svc.Thing(ctx)
}

func (r *queryResolver) GatedBoundThenChecked(ctx context.Context, id string) (*model.Thing, error) {
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errInvalidID("id", err)
	}
	err = auth.RequireChannelPermission(ctx, r.AdminSvc, "x.view", uid, nil)
	if err != nil {
		return nil, err
	}
	return r.svc.Thing(ctx)
}

func (r *queryResolver) GatedThroughHelper(ctx context.Context) (*model.Thing, error) {
	if err := r.strictHelper(ctx); err != nil {
		return nil, err
	}
	return r.svc.Thing(ctx)
}

func (r *Resolver) strictHelper(ctx context.Context) error {
	if r.DevMode {
		return errors.New("refused")
	}
	return auth.RequirePermission(ctx, r.AdminSvc, "x.view")
}

func (r *queryResolver) GatedPassBranch(ctx context.Context) (*model.Thing, error) {
	if err := auth.RequirePermission(ctx, r.AdminSvc, "x.view"); err == nil {
		return r.svc.Thing(ctx)
	}
	return nil, nil
}

func (r *queryResolver) GatedDisjunction(ctx context.Context) (*model.Thing, error) {
	err := auth.RequirePermission(ctx, r.AdminSvc, "x.view")
	if err != nil || r.DevMode {
		return nil, err
	}
	return r.svc.Thing(ctx)
}
