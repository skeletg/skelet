package medialibrary

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/skeletg/skelet/apps/backend/core/internal/media"
)

const thumbMemoMax = 50_000

// Resized copies already seen in storage - one is written once and never changes, and if storage loses it the dashboard falls back to the original
type thumbMemo struct {
	seen sync.Map
	n    atomic.Int64
}

func (m *thumbMemo) has(key string) bool {
	_, ok := m.seen.Load(key)
	return ok
}

func (m *thumbMemo) add(key string) {
	if _, loaded := m.seen.LoadOrStore(key, struct{}{}); !loaded && m.n.Add(1) > thumbMemoMax {
		m.seen.Clear()
		m.n.Store(0)
	}
}

// Storage's own address for a copy no wider than width when it is already there, else the path that makes it - and the original where a copy gains nothing
func (s *Service) ThumbnailURL(ctx context.Context, a Asset, width int) string {
	width = media.SnapWidth(width)
	if a.Status != AssetStatusReady || !media.CanResize(a.MimeType) || a.Width == nil || int(*a.Width) <= width || !s.storage.HasPublicBase() {
		return s.storage.PublicURL(a.ObjectKey)
	}
	copyKey := media.DerivedKey(a.ObjectKey, width, 0, media.CanEncodeWebP)
	if !s.thumbs.has(copyKey) {
		if _, _, ok, _ := s.storage.Head(ctx, copyKey); !ok {
			return media.TransformPath(media.AdminTransformRoute, a.ObjectKey, width)
		}
		s.thumbs.add(copyKey)
	}
	return s.storage.PublicURL(copyKey)
}
