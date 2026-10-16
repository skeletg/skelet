package delivery

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type failingSource struct{}

func (failingSource) Search(context.Context, PointQuery) ([]Point, error) {
	return nil, errors.New("POST https://lockers.example/api?key=s3cret: 502 upstream body")
}

func (failingSource) Get(_ context.Context, _ uuid.UUID, id string) (Point, error) {
	if id == "gone" {
		return Point{}, ErrPointNotFound
	}
	return Point{}, errors.New("dial tcp: connection refused")
}

// failingOption is a locker option whose points come from failingSource
func failingOption(t *testing.T) Option {
	t.Helper()
	RegisterPointSource(PointSourceRegistration{Slug: "test-failing", Kinds: []string{KindParcelLocker}, Source: failingSource{}})
	t.Cleanup(resetForTest)
	return Option{ID: uuid.New(), ChannelID: uuid.New(), Kind: KindParcelLocker, PointSource: "test-failing"}
}

// A source's failure reaches the shopper as "points are unavailable" and nothing more - its text can carry a provider's URL, key or response body
func TestSearchPointsHidesTheSourceError(t *testing.T) {
	o := failingOption(t)
	_, err := (&Service{}).SearchPoints(context.Background(), o, PointQuery{})
	if !errors.Is(err, ErrSourceUnavailable) {
		t.Fatalf("SearchPoints = %v, want ErrSourceUnavailable", err)
	}
	if strings.Contains(err.Error(), "s3cret") {
		t.Fatal("the source's error text reached the caller")
	}
}

// A shopper's % or _ is searched for literally - unescaped, "%" alone listed every point
func TestLikeEscaperKeepsWildcardsLiteral(t *testing.T) {
	if got := likeEscaper.Replace(`50%_off\`); got != `50\%\_off\\` {
		t.Fatalf("escaped = %q", got)
	}
}

// A source that can't answer is unavailable, not a missing point - the shopper retries instead of picking another point that fails the same way
func TestGetPointTellsAnOutageFromAMissingPoint(t *testing.T) {
	o := failingOption(t)
	if _, err := (&Service{}).GetPoint(context.Background(), o, "any"); !errors.Is(err, ErrSourceUnavailable) {
		t.Errorf("outage = %v, want ErrSourceUnavailable", err)
	}
	if _, err := (&Service{}).GetPoint(context.Background(), o, "gone"); !errors.Is(err, ErrPointNotFound) {
		t.Errorf("missing point = %v, want ErrPointNotFound", err)
	}
}
