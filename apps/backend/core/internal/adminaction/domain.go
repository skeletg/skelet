// Package adminaction registers admin operations for the "Tools" surface - can't use init() like other registries since handlers need live service references, so domain packages call RegisterActions(svc) once main.go has built everything
package adminaction

import (
	"context"
	"errors"
	"time"
)

type ArgumentType string

const (
	ArgString ArgumentType = "string"
	ArgInt    ArgumentType = "int"
	ArgBool   ArgumentType = "bool"
	ArgSelect ArgumentType = "select"
)

type ArgumentOption struct {
	Value string
	Label string
}

type Argument struct {
	Name        string
	Label       string
	Description string
	Type        ArgumentType
	Required    bool
	Default     string
	Options     []ArgumentOption // ArgSelect only
}

type Handler func(ctx context.Context, args Args) (Result, error) // args are pre-validated against the schema before this runs, and a returned error just becomes success=false with the error text as Output

type Definition struct {
	Slug                    string // e.g. "config.prune_orphans"
	Name                    string
	Description             string // mention destructiveness here, admins read this
	Group                   string
	Permission              string
	RequireGlobalPermission bool // set when the blast radius is the whole installation - a channel-scoped grant of Permission must not be enough to run it
	Arguments               []Argument
	Handler                 Handler
	Timeout                 time.Duration // 0 -> 30s default
}

type Result struct {
	Output string
	Data   any
}

var (
	ErrUnknownAction = errors.New("unknown admin action")
	ErrInvalidArgs   = errors.New("invalid action arguments") // wrap with which arg failed
)
