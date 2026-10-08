package cms

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/urlrewrite"
)

// urlrewrite.Service's tx form - its cache invalidations and events wait for the returned flush, run once the tx commits
type deferredRewriter interface {
	WithTxDeferred(t *sql.Tx) (*urlrewrite.Service, func(context.Context))
}

func noFlush(context.Context) {}

func (s *core) rewritesIn(t *sql.Tx) (UrlRewriteSyncer, func(context.Context)) {
	if s.urlRewrites == nil {
		return nil, noFlush
	}
	if d, ok := s.urlRewrites.(deferredRewriter); ok {
		bound, flush := d.WithTxDeferred(t)
		return bound, flush
	}
	return s.urlRewrites, noFlush
}

// Moves page rows and their url_rewrites in one tx - the rewrites' side effects fire only after commit
func (s *core) inURLTx(ctx context.Context, fn func(r *postgresRepository, rw UrlRewriteSyncer) error) error {
	flush := noFlush
	err := s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		rw, f := s.rewritesIn(t)
		flush = f
		return fn(s.txRepo(t), rw)
	})
	if err != nil {
		return err
	}
	flush(ctx)
	return nil
}

// CMS page URL composition - a page has a channel-agnostic base_path, its hierarchical slug path, and one or more public canonicals produced by prefixing the page type's root slug
// A per-channel canonical is only persisted for channels whose effective root makes the path DIFFER from the global one; everywhere else the global canonical serves the channel, through channel-first-then-global resolution

func joinRoot(root, base string) string {
	root = strings.Trim(strings.TrimSpace(root), "/")
	if root == "" {
		return base
	}
	if base == "" {
		return root
	}
	return root + "/" + base
}

// A page's channel-agnostic hierarchical path from its slug and parent - top-level is the bare slug, nested is parent.BasePath plus the slug
func (s *pageService) pageBasePath(ctx context.Context, slug string, parentID *uuid.UUID) (string, error) {
	if parentID == nil {
		return slug, nil
	}
	parent, err := s.repo.GetPageByID(ctx, *parentID)
	if err != nil {
		return "", err
	}
	base := parent.BasePath + "/" + slug
	if strings.Count(base, "/") >= maxPageDepth {
		return "", ErrPageTooDeep
	}
	return base, nil
}

func publicPaths(basePath string, pt PageType) []string {
	want := desiredPageURLs(basePath, pt)
	out := make([]string, len(want))
	for i, sp := range want {
		out[i] = "/" + sp.path
	}
	return out
}

type scopedPath struct {
	channelID *uuid.UUID
	path      string
}

func channelKey(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return id.String()
}

// The canonical paths a page should have given its type - always the global path, plus one per channel override whose path differs from it
func desiredPageURLs(basePath string, pt PageType) []scopedPath {
	global := joinRoot(pt.RootSlug, basePath)
	out := []scopedPath{{channelID: nil, path: global}}
	for _, cr := range pt.ChannelRoots {
		p := joinRoot(cr.RootSlug, basePath)
		if p == global {
			continue // global canonical already serves this channel
		}
		ch := cr.ChannelID
		out = append(out, scopedPath{channelID: &ch, path: p})
	}
	return out
}

type pathWriter interface {
	UpdatePagePaths(ctx context.Context, id uuid.UUID, basePath, urlPath string) error
}

func checkPageURLs(ctx context.Context, rw UrlRewriteSyncer, pageID uuid.UUID, want []scopedPath) error {
	for _, sp := range want {
		conflict, err := rw.PathConflictForChannel(ctx, sp.path, entityCMSPage, pageID, sp.channelID)
		if err != nil {
			return err
		}
		if conflict {
			return ErrURLPathConflict
		}
	}
	return nil
}

// Reconciles a page's stored paths and url_rewrites canonicals to the desired set, leaving 301s from any changed or removed paths
func syncPageURLs(ctx context.Context, repo pathWriter, rw UrlRewriteSyncer, pageID uuid.UUID, basePath string, pt PageType, old []scopedPath) error {
	want := desiredPageURLs(basePath, pt)
	global := want[0].path
	if rw == nil {
		return repo.UpdatePagePaths(ctx, pageID, basePath, global)
	}
	if err := checkPageURLs(ctx, rw, pageID, want); err != nil {
		return err
	}
	if err := repo.UpdatePagePaths(ctx, pageID, basePath, global); err != nil {
		return err
	}

	oldByChannel := make(map[string]string, len(old))
	for _, sp := range old {
		oldByChannel[channelKey(sp.channelID)] = sp.path
	}
	wantKeys := make(map[string]bool, len(want))

	for _, sp := range want {
		key := channelKey(sp.channelID)
		wantKeys[key] = true
		if err := rw.SyncCanonicalForChannel(ctx, entityCMSPage, pageID, sp.path, sp.channelID); err != nil {
			if errors.Is(err, urlrewrite.ErrPathConflict) {
				return ErrURLPathConflict
			}
			return err
		}
		if prev, ok := oldByChannel[key]; ok && prev != sp.path {
			if err := redirectFrom(ctx, rw, prev, pageID, sp.channelID); err != nil {
				return err
			}
		}
	}

	// Channels that had a distinct canonical but no longer do, because the root override was removed or now equals global - drop the stale row and 301 its old path to the global canonical
	for _, sp := range old {
		key := channelKey(sp.channelID)
		if key == "" || wantKeys[key] {
			continue
		}
		if err := rw.DeleteCanonicalForChannel(ctx, entityCMSPage, pageID, sp.channelID); err != nil {
			return err
		}
		if err := redirectFrom(ctx, rw, sp.path, pageID, sp.channelID); err != nil {
			return err
		}
	}
	return nil
}

// A path someone else already holds owes this page no redirect - checked first, since a refused insert would abort the surrounding tx
func redirectFrom(ctx context.Context, rw UrlRewriteSyncer, from string, pageID uuid.UUID, channelID *uuid.UUID) error {
	taken, err := rw.PathConflictForChannel(ctx, from, entityCMSPage, pageID, channelID)
	if err != nil || taken {
		return err
	}
	return rw.CreateRedirectForChannel(ctx, from, entityCMSPage, pageID, 301, channelID)
}
