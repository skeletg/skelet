// Package experiment is the frontend A/B engine, GrowthBook-inspired - it owns experiment definitions and the deterministic hash-bucket assignment shared with the storefront SDK, using the same FNV-1a so a visitor sticks to one variation across renders
package experiment

import (
	"hash/fnv"
	"time"

	"github.com/google/uuid"
)

// Statuses (validated in Go)
const (
	StatusDraft   = "draft"
	StatusRunning = "running"
	StatusStopped = "stopped"
)

// One arm of an experiment - Value is the payload the storefront renders, a flag string, copy variant or URL, interpreted by the page
type Variation struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Weight int    `json:"weight"`
	Value  string `json:"value"`
}

// Experiment is an admin-defined split test
type Experiment struct {
	ID          uuid.UUID
	ChannelID   uuid.UUID
	Key         string
	Name        string
	Status      string
	Coverage    int // 0..100 - % of visitors entered
	GoalEvent   string
	SegmentSlug string // optional targeting: only customers in this segment are entered ("" = all)
	Variations  []Variation
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Maps (seed, anonID) to a deterministic float in [0,1) through FNV-1a-32, identical to the storefront SDK's fnv1a, so client and server agree on assignment
func bucket(seed, anonID string) float64 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed + anonID))
	return float64(h.Sum32()%10000) / 10000.0
}

// The variation a visitor is bucketed into, or inExperiment=false when the coverage gate excludes them - deterministic and sticky on anonID, with a separate coverage seed decorrelating the gate from the variation split
func (e Experiment) Assign(anonID string) (variation Variation, inExperiment bool) {
	if e.Coverage <= 0 || len(e.Variations) == 0 {
		return Variation{}, false
	}
	if e.Coverage < 100 && bucket(e.Key+":coverage", anonID) >= float64(e.Coverage)/100 {
		return Variation{}, false
	}
	total := 0
	for _, v := range e.Variations {
		if v.Weight > 0 {
			total += v.Weight
		}
	}
	if total <= 0 {
		return e.Variations[0], true // no weights → everyone gets the control
	}
	target := bucket(e.Key, anonID) * float64(total)
	acc := 0.0
	for _, v := range e.Variations {
		acc += float64(v.Weight)
		if target < acc {
			return v, true
		}
	}
	return e.Variations[len(e.Variations)-1], true
}
