package cms

import (
	"context"
	"strings"

	"github.com/google/uuid"
)

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
	return parent.BasePath + "/" + slug, nil
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

// Reconciles a page's stored paths and url_rewrites canonicals to the desired set, leaving 301s from any changed or removed paths
func (s *core) syncPageURLs(ctx context.Context, pageID uuid.UUID, basePath string, pt PageType, old []scopedPath) (string, error) {
	want := desiredPageURLs(basePath, pt)
	global := want[0].path

	// Conflict-check every scope before claiming any path
	for _, sp := range want {
		conflict, err := s.urlRewrites.PathConflictForChannel(ctx, sp.path, entityCMSPage, pageID, sp.channelID)
		if err != nil {
			return "", err
		}
		if conflict {
			return "", ErrURLPathConflict
		}
	}

	if err := s.repo.UpdatePagePaths(ctx, pageID, basePath, global); err != nil {
		return "", err
	}

	oldByChannel := make(map[string]string, len(old))
	for _, sp := range old {
		oldByChannel[channelKey(sp.channelID)] = sp.path
	}
	wantKeys := make(map[string]bool, len(want))

	for _, sp := range want {
		key := channelKey(sp.channelID)
		wantKeys[key] = true
		if err := s.urlRewrites.SyncCanonicalForChannel(ctx, entityCMSPage, pageID, sp.path, sp.channelID); err != nil {
			return "", err
		}
		if prev, ok := oldByChannel[key]; ok && prev != sp.path {
			_ = s.urlRewrites.CreateRedirectForChannel(ctx, prev, entityCMSPage, pageID, 301, sp.channelID)
		}
	}

	// Channels that had a distinct canonical but no longer do, because the root override was removed or now equals global - drop the stale row and 301 its old path to the global canonical
	for _, sp := range old {
		key := channelKey(sp.channelID)
		if key == "" || wantKeys[key] {
			continue
		}
		_ = s.urlRewrites.DeleteCanonicalForChannel(ctx, entityCMSPage, pageID, sp.channelID)
		_ = s.urlRewrites.CreateRedirectForChannel(ctx, sp.path, entityCMSPage, pageID, 301, sp.channelID)
	}

	return global, nil
}
