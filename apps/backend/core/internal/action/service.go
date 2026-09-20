package action

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const defaultTimeout = 30 * time.Second

type Service struct{}

func NewService() *Service { return &Service{} }

// Permission is NOT checked here - the resolver checks it before calling Run
// actingAdminID is audit-only, pass uuid.Nil for a superuser or unknown caller
func (s *Service) Run(ctx context.Context, slug string, rawArgs map[string]any, actingAdminID uuid.UUID) (Result, error) {
	def, ok := Find(slug)
	if !ok {
		return Result{}, ErrUnknownAction
	}
	validated, err := validateArgs(def.Arguments, rawArgs)
	if err != nil {
		return Result{}, fmt.Errorf("%w: %w", ErrInvalidArgs, err)
	}

	timeout := def.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	result, runErr := def.Handler(runCtx, Args{raw: validated})
	duration := time.Since(start)

	entry, _ := json.Marshal(map[string]any{
		"type":        "admin_action",
		"slug":        slug,
		"admin_id":    actingAdminID.String(),
		"duration_ms": duration.Milliseconds(),
		"success":     runErr == nil,
	})
	log.Printf("[admin_action] %s", entry)

	return result, runErr
}

func validateArgs(schema []Argument, raw map[string]any) (map[string]any, error) {
	allowed := make(map[string]Argument, len(schema))
	for _, a := range schema {
		allowed[a.Name] = a
	}
	for name := range raw {
		if _, ok := allowed[name]; !ok {
			return nil, fmt.Errorf("unknown argument %q", name)
		}
	}

	out := make(map[string]any, len(schema))
	for _, a := range schema {
		rawVal, present := raw[a.Name]
		if !present {
			if a.Required {
				return nil, fmt.Errorf("missing required argument %q", a.Name)
			}
			if a.Default != "" {
				coerced, err := coerceDefault(a)
				if err != nil {
					return nil, err
				}
				out[a.Name] = coerced
			}
			continue
		}
		coerced, err := coerceValue(a, rawVal)
		if err != nil {
			return nil, err
		}
		out[a.Name] = coerced
	}
	return out, nil
}

func coerceValue(a Argument, v any) (any, error) {
	switch a.Type {
	case ArgString:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("argument %q: expected string", a.Name)
		}
		return s, nil
	case ArgInt:
		switch x := v.(type) {
		case float64:
			return int64(x), nil
		case int:
			return int64(x), nil
		case int64:
			return x, nil
		case string:
			n, err := strconv.ParseInt(x, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("argument %q: %w", a.Name, err)
			}
			return n, nil
		}
		return nil, fmt.Errorf("argument %q: expected int", a.Name)
	case ArgBool:
		b, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("argument %q: expected bool", a.Name)
		}
		return b, nil
	case ArgSelect:
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("argument %q: expected string (select)", a.Name)
		}
		for _, opt := range a.Options {
			if opt.Value == s {
				return s, nil
			}
		}
		return nil, fmt.Errorf("argument %q: %q is not an allowed value", a.Name, s)
	}
	return nil, fmt.Errorf("argument %q: unknown type %q", a.Name, a.Type)
}

func coerceDefault(a Argument) (any, error) {
	switch a.Type {
	case ArgString, ArgSelect:
		return a.Default, nil
	case ArgInt:
		n, err := strconv.ParseInt(a.Default, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("argument %q default: %w", a.Name, err)
		}
		return n, nil
	case ArgBool:
		b, err := strconv.ParseBool(a.Default)
		if err != nil {
			return nil, fmt.Errorf("argument %q default: %w", a.Name, err)
		}
		return b, nil
	}
	return nil, fmt.Errorf("argument %q: unknown type %q", a.Name, a.Type)
}
