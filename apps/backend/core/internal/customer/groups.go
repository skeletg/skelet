package customer

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/internal/events"
)

func (s *Service) CreateGroup(ctx context.Context, input CreateCustomerGroupInput) (CustomerGroup, error) {
	if err := validateGroupSlug(input.Slug); err != nil {
		return CustomerGroup{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return CustomerGroup{}, ErrGroupNameRequired
	}
	return s.repo.CreateGroup(ctx, input)
}

func (s *Service) UpdateGroup(ctx context.Context, id uuid.UUID, input UpdateCustomerGroupInput) (CustomerGroup, error) {
	if input.Name != nil && strings.TrimSpace(*input.Name) == "" {
		return CustomerGroup{}, ErrGroupNameRequired
	}
	return s.repo.UpdateGroup(ctx, id, input)
}

// ErrGroupInUse if it has members, ErrCannotDeleteDefaultGroup if it's the current default
func (s *Service) DeleteGroup(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteGroup(ctx, id)
}

func (s *Service) GetGroup(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	return s.repo.GetGroupByID(ctx, id)
}

func (s *Service) GetGroupBySlug(ctx context.Context, slug string) (CustomerGroup, error) {
	return s.repo.GetGroupBySlug(ctx, slug)
}

func (s *Service) GetDefaultGroup(ctx context.Context) (CustomerGroup, error) {
	return s.repo.GetDefaultGroup(ctx)
}

// Populates each group's member count
func (s *Service) ListGroups(ctx context.Context) ([]CustomerGroup, error) {
	return s.repo.ListGroups(ctx)
}

// No-op when it's already the default, otherwise a swap in one transaction - ErrDefaultGroupConflict when another swap won the race
func (s *Service) SetDefaultGroup(ctx context.Context, id uuid.UUID) (CustomerGroup, error) {
	current, err := s.repo.GetGroupByID(ctx, id)
	if err != nil {
		return CustomerGroup{}, err
	}
	if current.IsDefault {
		return current, nil
	}
	var updated CustomerGroup
	err = s.atomically(ctx, func(r *postgresRepository) ([]events.Event, error) {
		var err error
		updated, err = r.SetDefaultGroup(ctx, id)
		return nil, err
	})
	return updated, err
}

// Verifies the group exists first so a bogus id gets a clean error
func (s *Service) SetCustomerGroup(ctx context.Context, customerID, groupID uuid.UUID) (Customer, error) {
	if _, err := s.repo.GetGroupByID(ctx, groupID); err != nil {
		return Customer{}, err
	}
	return s.adminSet(ctx, customerID, []string{FieldCustomerGroup}, func(r *postgresRepository) error {
		return r.SetCustomerGroup(ctx, customerID, groupID)
	})
}

// Keeps group slugs usable as stable identifiers in GraphQL inputs and future pricing rules
func validateGroupSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("%w: slug is required", ErrInvalidGroupSlug)
	}
	if slug != strings.ToLower(slug) {
		return fmt.Errorf("%w: slug must be lowercase", ErrInvalidGroupSlug)
	}
	if strings.ContainsAny(slug, " \t\n") {
		return fmt.Errorf("%w: slug must not contain whitespace", ErrInvalidGroupSlug)
	}
	return nil
}
