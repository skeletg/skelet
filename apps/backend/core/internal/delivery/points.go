package delivery

import (
	"context"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// ManualPoint is a pickup point (locker, parcel shop, PO box) the admin entered for a channel - the manual-points source
type ManualPoint struct {
	ID           uuid.UUID
	Kind         string
	Name         string
	Address      Address
	Instructions string
	Hours        string
	Active       bool
	SortOrder    int32
}

func (p ManualPoint) point() Point {
	return Point{ID: p.ID.String(), Source: ManualPointSource, Name: p.Name, Address: p.Address, Instructions: p.Instructions, Hours: p.Hours}
}

// manualPoints serves the admin's pickup points from the database - registered at init like the core kinds, and pointed at the service's repository once it is built
type manualPoints struct{ repo Repository }

var manualSource = &manualPoints{}

func (m *manualPoints) Search(ctx context.Context, q PointQuery) ([]Point, error) {
	if m.repo == nil {
		return nil, ErrSourceUnavailable
	}
	rows, err := m.repo.SearchPoints(ctx, q.ChannelID, q.Kind, q.Near, strings.TrimSpace(q.Search), q.Limit)
	if err != nil {
		return nil, err
	}
	out := make([]Point, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.point())
	}
	return out, nil
}

func (m *manualPoints) Get(ctx context.Context, channelID uuid.UUID, id string) (Point, error) {
	if m.repo == nil {
		return Point{}, ErrSourceUnavailable
	}
	pid, err := uuid.Parse(id)
	if err != nil {
		return Point{}, ErrPointNotFound
	}
	p, err := m.repo.GetPoint(ctx, channelID, pid)
	if err != nil {
		return Point{}, err
	}
	return p.point(), nil
}

func registerManualPointSource() {
	RegisterPointSource(PointSourceRegistration{
		Slug: ManualPointSource, Label: "Pickup points listed in checkout settings", Kinds: []string{KindParcelLocker}, Source: manualSource,
	})
}

func (s *Service) ListPoints(ctx context.Context, channelID uuid.UUID) ([]ManualPoint, error) {
	return s.repo.ListPoints(ctx, channelID)
}

// SavePoints replaces the channel's pickup points with the list given - a point without an id is new
func (s *Service) SavePoints(ctx context.Context, channelID uuid.UUID, points []ManualPoint) ([]ManualPoint, error) {
	for i := range points {
		p := &points[i]
		if p.ID == uuid.Nil {
			p.ID = uuid.Must(uuid.NewV7())
		}
		if p.Kind == "" {
			p.Kind = KindParcelLocker
		}
		p.Name = strings.TrimSpace(p.Name)
		p.Address.CountryCode = strings.ToUpper(strings.TrimSpace(p.Address.CountryCode))
		if p.Kind != KindParcelLocker {
			return nil, apperr.New("DELIVERY_POINT_KIND", apperr.ClassValidation, "only PO boxes and lockers are listed here - stores come from inventory locations")
		}
		if p.Name == "" || strings.TrimSpace(p.Address.Street1) == "" || strings.TrimSpace(p.Address.City) == "" ||
			strings.TrimSpace(p.Address.PostalCode) == "" || len(p.Address.CountryCode) != 2 {
			return nil, apperr.New("DELIVERY_POINT_INVALID", apperr.ClassValidation, "a PO box needs a name, street, city, postcode and a two-letter country")
		}
		p.SortOrder = int32(i)
	}
	return s.repo.ReplacePoints(ctx, channelID, points)
}
