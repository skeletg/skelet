package sdk

import (
	"regexp"
	"slices"
	"sync"
)

// One hook's registrations, in the order plugins made them
type registry[T any] struct {
	mu   sync.Mutex
	list []T
}

// Panics with why when clash matches one already there
func (r *registry[T]) add(v T, clash func(T) bool, why string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if clash != nil && slices.ContainsFunc(r.list, clash) {
		panic(why)
	}
	r.list = append(r.list, v)
}

func (r *registry[T]) all() []T {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.list)
}

func (r *registry[T]) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.list = nil
}

// The shape of every key a plugin names for a hook - a checkout field, a stage, a data resource
var hookKey = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)

const hookKeyRule = " must be lowercase letters, digits and underscores, starting with a letter"

// Key is a key this plugin named as core keeps it, "<plugin>:<key>" - so two plugins asking the same thing never collide
func (pl *Plugin) Key(key string) string { return pl.slug + ":" + key }
