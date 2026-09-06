package review

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"maps"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/internal/ai"
	"github.com/skeletg/skelet/apps/backend/internal/config"
	"github.com/skeletg/skelet/apps/backend/internal/events"
	"github.com/skeletg/skelet/apps/backend/internal/tx"
)

// Resolves the global default content locale - localized question reads fall back to it when the requested locale has no content row
type DefaultLocaleProvider interface {
	DefaultLocale(ctx context.Context) (string, error)
}

// Wraps multi-row operations in transactions, so a submit writes header, answers and media atomically, and emits events - optional deps attach through SetX so main.go wiring stays linear
type Service struct {
	repo *postgresRepository
	tx   *tx.Manager
	// cfg optional - nil (tests, CLI) uses strict defaults (auth + verified purchase required)
	cfg *config.Service
	// bus optional - nil (CLI, tests) means submit/approve/reject publish nothing
	bus *events.Bus
	// defaultLocale optional - nil skips the default-locale fallback
	defaultLocale DefaultLocaleProvider
	// ai optional - nil (or AI disabled) makes classify/summary/reply-draft inert
	ai *ai.Service
	// remoteAggs sources aggregates from an external platform when wired (remote.go)
	remoteAggs RemoteAggregateSource
}

func NewService(repo *postgresRepository, txMgr *tx.Manager) *Service {
	return &Service{repo: repo, tx: txMgr}
}

// SetConfigService is optional - omitting it leaves the service on strict defaults
func (s *Service) SetConfigService(c *config.Service) {
	s.cfg = c
}

// SetEventBus is optional - omitting it makes Submit/Approve/Reject silent
func (s *Service) SetEventBus(b *events.Bus) {
	s.bus = b
}

// SetDefaultLocaleProvider is optional - omitting it disables the default-locale fallback
func (s *Service) SetDefaultLocaleProvider(p DefaultLocaleProvider) {
	s.defaultLocale = p
}

// SetAIService is optional - omitting it (or AI disabled) makes the AI features inert
func (s *Service) SetAIService(a *ai.Service) {
	s.ai = a
}

// minReviewsForSummary - below this a product gets no AI summary (too few opinions, not worth the cost)
const minReviewsForSummary = 3

func (s *Service) aiReady(ctx context.Context) bool {
	return s.ai != nil && s.ai.Available(ctx)
}

// resolveDefaultLocale returns the global default locale, or "" with no provider / on error
func (s *Service) resolveDefaultLocale(ctx context.Context) string {
	if s.defaultLocale == nil {
		return ""
	}
	code, err := s.defaultLocale.DefaultLocale(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(code)
}

// Policy is the current review-submission knob state, so resolvers can short-circuit early
type Policy struct {
	RequireAuthentication   bool
	RequireVerifiedPurchase bool
	// RequireAuthenticationToVote - true (default) only signed-in vote, false allows cookie-debounced anon
	RequireAuthenticationToVote bool
}

// Identifies who is voting through the policy chain - customerID is the signed-in customer, rawAnonToken the header token, and a nil secret disables anon voting
// ErrVoteAuthRequired when nothing usable is present and anon voting is off, or the anon token doesn't validate - Centralised so the resolver and dataloader can't drift
func (s *Service) ResolveVoter(ctx context.Context, customerID *uuid.UUID, rawAnonToken string, secret []byte) (Voter, error) {
	if customerID != nil {
		return Voter{CustomerID: customerID}, nil
	}
	if s.ReadPolicy(ctx).RequireAuthenticationToVote {
		return Voter{}, ErrVoteAuthRequired
	}
	if len(secret) == 0 {
		return Voter{}, ErrVoteAuthRequired
	}
	raw := strings.TrimSpace(rawAnonToken)
	if raw == "" {
		return Voter{}, ErrVoteAuthRequired
	}
	tok, err := ParseVoterToken(secret, raw)
	if err != nil {
		return Voter{}, ErrVoteAuthRequired
	}
	return Voter{AnonToken: &tok}, nil
}

// ReadPolicy reads every knob from config, falling back to strict defaults on any error
func (s *Service) ReadPolicy(ctx context.Context) Policy {
	p := Policy{RequireAuthentication: true, RequireVerifiedPurchase: true, RequireAuthenticationToVote: true}
	if s.cfg == nil {
		return p
	}
	if v, err := s.cfg.GetBool(ctx, pathRequireAuthentication, config.DefaultScope); err == nil {
		p.RequireAuthentication = v
	}
	if v, err := s.cfg.GetBool(ctx, pathRequireVerifiedPurchase, config.DefaultScope); err == nil {
		p.RequireVerifiedPurchase = v
	}
	if v, err := s.cfg.GetBool(ctx, pathRequireAuthenticationToVote, config.DefaultScope); err == nil {
		p.RequireAuthenticationToVote = v
	}
	return p
}

// --- Categories ---

func (s *Service) CreateCategory(ctx context.Context, in CreateCategoryInput) (Category, error) {
	in.Slug = strings.TrimSpace(in.Slug)
	if in.Slug == "" {
		return Category{}, fmt.Errorf("review: slug required")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Category{}, fmt.Errorf("review: name required")
	}
	return s.repo.CreateCategory(ctx, in)
}

func (s *Service) GetCategory(ctx context.Context, id uuid.UUID) (Category, error) {
	return s.repo.GetCategory(ctx, id)
}

func (s *Service) ListCategories(ctx context.Context) ([]Category, error) {
	return s.repo.ListCategories(ctx)
}

func (s *Service) ListActiveCategories(ctx context.Context) ([]Category, error) {
	return s.repo.ListActiveCategories(ctx)
}

func (s *Service) UpdateCategory(ctx context.Context, in UpdateCategoryInput) (Category, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return Category{}, fmt.Errorf("review: name required")
	}
	return s.repo.UpdateCategory(ctx, in)
}

func (s *Service) DeleteCategory(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteCategory(ctx, id)
}

// A moderator override of a review's categories, empty clearing them - slugs are validated against the live taxonomy so unknowns can't slip in
func (s *Service) SetReviewCategories(ctx context.Context, reviewID uuid.UUID, slugs []string) (Review, error) {
	cats, err := s.repo.ListCategories(ctx)
	if err != nil {
		return Review{}, err
	}
	known := make(map[string]bool, len(cats))
	for _, c := range cats {
		known[c.Slug] = true
	}
	clean := make([]string, 0, len(slugs))
	seen := map[string]bool{}
	for _, sl := range slugs {
		sl = strings.TrimSpace(sl)
		if sl == "" || seen[sl] || !known[sl] {
			continue
		}
		seen[sl] = true
		clean = append(clean, sl)
	}
	return s.repo.SetReviewCategories(ctx, reviewID, clean)
}

// --- Moderation tags (abstract moderation-concern taxonomy) ---

func (s *Service) CreateModerationTag(ctx context.Context, in CreateModerationTagInput) (ModerationTag, error) {
	in.Slug = strings.TrimSpace(in.Slug)
	if in.Slug == "" {
		return ModerationTag{}, fmt.Errorf("review: slug required")
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return ModerationTag{}, fmt.Errorf("review: name required")
	}
	return s.repo.CreateModerationTag(ctx, in)
}

func (s *Service) GetModerationTag(ctx context.Context, id uuid.UUID) (ModerationTag, error) {
	return s.repo.GetModerationTag(ctx, id)
}

func (s *Service) ListModerationTags(ctx context.Context) ([]ModerationTag, error) {
	return s.repo.ListModerationTags(ctx)
}

func (s *Service) ListActiveModerationTags(ctx context.Context) ([]ModerationTag, error) {
	return s.repo.ListActiveModerationTags(ctx)
}

func (s *Service) UpdateModerationTag(ctx context.Context, in UpdateModerationTagInput) (ModerationTag, error) {
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" {
		return ModerationTag{}, fmt.Errorf("review: name required")
	}
	return s.repo.UpdateModerationTag(ctx, in)
}

func (s *Service) DeleteModerationTag(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteModerationTag(ctx, id)
}

// A moderator override of a review's tags, empty clearing them, validated against the live taxonomy - independent of the AI label
func (s *Service) SetReviewModerationTags(ctx context.Context, reviewID uuid.UUID, slugs []string) (Review, error) {
	tags, err := s.repo.ListModerationTags(ctx)
	if err != nil {
		return Review{}, err
	}
	known := make(map[string]bool, len(tags))
	for _, t := range tags {
		known[t.Slug] = true
	}
	clean := make([]string, 0, len(slugs))
	seen := map[string]bool{}
	for _, s := range slugs {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] || !known[s] {
			continue
		}
		seen[s] = true
		clean = append(clean, s)
	}
	return s.repo.SetReviewModerationTags(ctx, reviewID, clean)
}

// --- Questions ---

func (s *Service) CreateQuestion(ctx context.Context, in CreateQuestionInput) (Question, error) {
	in.Slug = strings.TrimSpace(in.Slug)
	if in.Slug == "" {
		return Question{}, fmt.Errorf("review: slug required")
	}
	if !in.InputType.IsValid() {
		return Question{}, ErrInvalidInputType
	}
	return s.repo.CreateQuestion(ctx, in)
}

func (s *Service) GetQuestion(ctx context.Context, id uuid.UUID) (Question, error) {
	return s.repo.GetQuestion(ctx, id)
}

// The batched DataLoader lookup - AllContent isn't hydrated, since translations stay on the single-id GetQuestion path
func (s *Service) GetQuestionsByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]Question, error) {
	return s.repo.GetQuestionsByIDs(ctx, ids)
}

func (s *Service) ListQuestions(ctx context.Context) ([]Question, error) {
	return s.repo.ListQuestions(ctx)
}

func (s *Service) UpdateQuestion(ctx context.Context, in UpdateQuestionInput) (Question, error) {
	return s.repo.UpdateQuestion(ctx, in)
}

func (s *Service) DeleteQuestion(ctx context.Context, id uuid.UUID) error {
	return s.repo.DeleteQuestion(ctx, id)
}

func (s *Service) UpsertQuestionContent(ctx context.Context, in QuestionContentInput) (QuestionContent, error) {
	in.Locale = strings.TrimSpace(in.Locale)
	if in.Locale == "" {
		return QuestionContent{}, fmt.Errorf("review: locale required")
	}
	if strings.TrimSpace(in.Label) == "" {
		return QuestionContent{}, fmt.Errorf("review: label required")
	}
	return s.repo.UpsertQuestionContent(ctx, in)
}

// --- Assignments ---

func (s *Service) AssignQuestionToProductType(ctx context.Context, productTypeID, questionID uuid.UUID, sortOrder int32) error {
	return s.repo.AssignQuestion(ctx, productTypeID, questionID, sortOrder)
}

func (s *Service) UnassignQuestionFromProductType(ctx context.Context, productTypeID, questionID uuid.UUID) error {
	_, err := s.repo.UnassignQuestion(ctx, productTypeID, questionID)
	return err
}

func (s *Service) ListAssignedQuestions(ctx context.Context, productTypeID uuid.UUID) ([]Question, error) {
	questions, err := s.repo.ListAssignedQuestions(ctx, productTypeID)
	if err != nil {
		return nil, err
	}
	return questions, nil
}

// Loads the assigned questions and hydrates each with its localized label and help_text, falling back to the global default locale when the requested one has no row
// A question with no content row in either falls back to its slug, so the storefront never renders a blank prompt
func (s *Service) ListAssignedQuestionsLocalized(ctx context.Context, productTypeID uuid.UUID, locale string) ([]Question, error) {
	questions, err := s.repo.ListAssignedQuestions(ctx, productTypeID)
	if err != nil {
		return nil, err
	}
	if len(questions) == 0 {
		return questions, nil
	}
	ids := make([]uuid.UUID, len(questions))
	for i, q := range questions {
		ids[i] = q.ID
	}
	contents, err := s.repo.ListQuestionContentByIDs(ctx, ids, locale)
	if err != nil {
		return nil, err
	}
	if def := s.resolveDefaultLocale(ctx); def != "" && def != locale {
		var missing []uuid.UUID
		for _, q := range questions {
			if _, ok := contents[q.ID]; !ok {
				missing = append(missing, q.ID)
			}
		}
		if len(missing) > 0 {
			fb, err := s.repo.ListQuestionContentByIDs(ctx, missing, def)
			if err != nil {
				return nil, err
			}
			maps.Copy(contents, fb)
		}
	}
	for i := range questions {
		if c, ok := contents[questions[i].ID]; ok {
			cc := c
			questions[i].Content = &cc
		} else {
			questions[i].Content = &QuestionContent{
				QuestionID: questions[i].ID,
				Locale:     locale,
				Label:      questions[i].Slug, // fallback so prompts are never blank
			}
		}
	}
	return questions, nil
}

// --- Reviews ---

// Creates or replaces a review in one transaction, flushing header, answers and media together
// With a CustomerID it's authenticated and replaces on resubmit through the unique partial index, and verified-purchase is computed and persisted regardless of policy, so a later toggle has a correct discriminator
// Without one it's a guest, allowed only when require_authentication is false - never verified, and with no stable identity, so resubmits are new rows
func (s *Service) SubmitReview(ctx context.Context, productTypeID uuid.UUID, in SubmitReviewInput) (Review, error) {
	policy := s.ReadPolicy(ctx)
	isGuest := in.CustomerID == nil

	if isGuest {
		if policy.RequireAuthentication {
			return Review{}, ErrGuestNotAllowed
		}
		in.GuestName = strings.TrimSpace(in.GuestName)
		if in.GuestName == "" {
			return Review{}, ErrGuestNameRequired
		}
		in.GuestEmail = strings.TrimSpace(in.GuestEmail)
	}

	// Server-side length backstop against the bypass-the-form path - trim again so direct callers are covered, not just the resolver edge
	in.Title = strings.TrimSpace(in.Title)
	in.Body = strings.TrimSpace(in.Body)
	if err := validateFieldLengths(in); err != nil {
		return Review{}, err
	}

	// Verified-purchase only applies to the authenticated path - guests are always !purchased
	var purchased bool
	if !isGuest {
		var err error
		purchased, err = s.repo.HasCustomerPurchased(ctx, *in.CustomerID, in.ProductID)
		if err != nil {
			return Review{}, err
		}
		if policy.RequireVerifiedPurchase && !purchased {
			return Review{}, ErrNotPurchased
		}
	}

	// Validate answers against the product type's assigned questions - unknown IDs are rejected so a customer can't answer questions not on this product type
	assigned, err := s.repo.ListAssignedQuestions(ctx, productTypeID)
	if err != nil {
		return Review{}, err
	}
	allowed := make(map[uuid.UUID]Question, len(assigned))
	for _, q := range assigned {
		allowed[q.ID] = q
	}
	provided := make(map[uuid.UUID]struct{}, len(in.Answers))
	for _, a := range in.Answers {
		q, ok := allowed[a.QuestionID]
		if !ok {
			return Review{}, ErrUnknownQuestion
		}
		provided[a.QuestionID] = struct{}{}
		// Type-specific shape check
		switch q.InputType {
		case InputTypeRating1To5:
			if a.RatingValue == nil || *a.RatingValue < 1 || *a.RatingValue > 5 {
				return Review{}, ErrAnswerOutOfRange
			}
		case InputTypeYesNo:
			if a.BoolValue == nil {
				return Review{}, fmt.Errorf("review: yes/no question %s missing answer", q.Slug)
			}
		case InputTypeFreeText:
			if strings.TrimSpace(a.TextValue) == "" {
				return Review{}, fmt.Errorf("review: free-text question %s missing answer", q.Slug)
			}
		}
	}
	// Required questions must all be answered
	for _, q := range assigned {
		if !q.IsRequired {
			continue
		}
		if _, ok := provided[q.ID]; !ok {
			return Review{}, fmt.Errorf("%w: %s", ErrMissingRequired, q.Slug)
		}
	}

	// Pre-commit hook, sync and able to abort or auto-moderate - status starts pending, a spam plugin can flip it through the pointer, and the mutated values flow into the row below
	proposedStatus := StatusPending
	var proposedTags []string
	if s.bus != nil {
		submitting := &Submitting{
			ProductID:  in.ProductID,
			ChannelID:  in.ChannelID,
			CustomerID: in.CustomerID,
			GuestEmail: in.GuestEmail,
			Locale:     in.Locale,
			Title:      in.Title,
			Body:       in.Body,
			Status:     &proposedStatus,
			Tags:       &proposedTags,
		}
		if err := s.bus.PublishSync(ctx, submitting); err != nil {
			return Review{}, fmt.Errorf("review rejected: %w", err)
		}
	}

	var out Review
	err = s.tx.InTransaction(ctx, func(t *sql.Tx) error {
		repo := s.repo.WithTx(t)
		var rev Review
		var err error
		if isGuest {
			// Guests can't be deduped (no stable identity) - each submission is a fresh row
			rev, err = repo.CreateReview(ctx, Review{
				ProductID:          in.ProductID,
				CustomerID:         nil,
				GuestName:          in.GuestName,
				GuestEmail:         in.GuestEmail,
				ChannelID:          in.ChannelID,
				Locale:             in.Locale,
				Title:              in.Title,
				Body:               in.Body,
				IsVerifiedPurchase: false,
				Status:             proposedStatus,
			})
			if err != nil {
				return err
			}
		} else {
			// Replace-on-resubmit - reset the header back to pending for re-moderation and wipe the old answers and media, so the new submission is the only state
			existing, lookupErr := repo.GetReviewByCustomerAndProduct(ctx, *in.CustomerID, in.ProductID)
			switch {
			case lookupErr == nil:
				rev, err = repo.UpdateReviewHeader(ctx, existing.ID, in.Title, in.Body)
				if err != nil {
					return err
				}
				if err := repo.DeleteAnswers(ctx, rev.ID); err != nil {
					return err
				}
				if err := repo.DeleteMedia(ctx, rev.ID); err != nil {
					return err
				}
			case errors.Is(lookupErr, ErrNotFound):
				rev, err = repo.CreateReview(ctx, Review{
					ProductID:          in.ProductID,
					CustomerID:         in.CustomerID,
					ChannelID:          in.ChannelID,
					Locale:             in.Locale,
					Title:              in.Title,
					Body:               in.Body,
					IsVerifiedPurchase: purchased,
					Status:             proposedStatus,
				})
				if err != nil {
					return err
				}
			default:
				return lookupErr
			}
		}
		for _, a := range in.Answers {
			if err := repo.UpsertAnswer(ctx, Answer{
				ReviewID:    rev.ID,
				QuestionID:  a.QuestionID,
				RatingValue: a.RatingValue,
				BoolValue:   a.BoolValue,
				TextValue:   a.TextValue,
			}); err != nil {
				return err
			}
		}
		for i, assetID := range in.MediaAssetIDs {
			if _, err := repo.InsertMedia(ctx, rev.ID, assetID, int32(i)); err != nil {
				return err
			}
		}
		out = rev
		return nil
	})
	if err != nil {
		return Review{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &Submitted{
			ReviewID:   out.ID,
			ProductID:  out.ProductID,
			ChannelID:  out.ChannelID,
			CustomerID: out.CustomerID,
			Status:     out.Status,
			Tags:       proposedTags,
		})
	}
	return out, nil
}

func (s *Service) GetReview(ctx context.Context, id uuid.UUID) (Review, error) {
	return s.repo.GetReview(ctx, id)
}

// GetReviewWithDetails returns the header plus answers + media in three round-trips
func (s *Service) GetReviewWithDetails(ctx context.Context, id uuid.UUID) (Review, error) {
	rev, err := s.repo.GetReview(ctx, id)
	if err != nil {
		return Review{}, err
	}
	answers, err := s.repo.ListAnswers(ctx, id)
	if err != nil {
		return Review{}, err
	}
	media, err := s.repo.ListMedia(ctx, id)
	if err != nil {
		return Review{}, err
	}
	rev.Answers = answers
	rev.Media = media
	return rev, nil
}

// Looks up the (customer, product) row, ErrNotFound when there is none - the storefront uses it for eligibility
func (s *Service) GetReviewByCustomerAndProduct(ctx context.Context, customerID, productID uuid.UUID) (Review, error) {
	return s.repo.GetReviewByCustomerAndProduct(ctx, customerID, productID)
}

// ListReviewsForProduct returns a product's approved reviews, hydrated with answers + media (no N+1)
func (s *Service) ListReviewsForProduct(ctx context.Context, productID uuid.UUID, limit, offset int32) ([]Review, int64, error) {
	rows, total, err := s.repo.ListApprovedForProduct(ctx, productID, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return s.hydrate(ctx, rows, total)
}

// The batched form - top-N approved reviews for many products in one windowed query, hydrated in a round-trip each, for the storefront DataLoader
// No per-product totals, since single-product paging uses ListReviewsForProduct
func (s *Service) ListReviewsForProductIDs(ctx context.Context, productIDs []uuid.UUID, limit, offset int32) (map[uuid.UUID][]Review, error) {
	byProduct, err := s.repo.ListApprovedForProductIDs(ctx, productIDs, limit, offset)
	if err != nil {
		return nil, err
	}
	if len(byProduct) == 0 {
		return byProduct, nil
	}
	// Collect every review id, hydrate answers + media in one batch each, attach back
	var ids []uuid.UUID
	for _, rows := range byProduct {
		for _, rv := range rows {
			ids = append(ids, rv.ID)
		}
	}
	answers, err := s.repo.ListAnswersByReviewIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	media, err := s.repo.ListMediaByReviewIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	for pid, rows := range byProduct {
		for i := range rows {
			rows[i].Answers = answers[rows[i].ID]
			rows[i].Media = media[rows[i].ID]
		}
		byProduct[pid] = rows
	}
	return byProduct, nil
}

func (s *Service) ListReviewsForModeration(ctx context.Context, filter ListReviewsFilter, limit, offset int32) ([]Review, int64, error) {
	rows, total, err := s.repo.ListForModeration(ctx, filter, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	return s.hydrate(ctx, rows, total)
}

// Per-status tallies for the moderation tiles, channel-scoped with nil for all - exact regardless of paging, so the tiles don't drift
func (s *Service) CountReviewsByStatus(ctx context.Context, channelIDs []uuid.UUID, productID *uuid.UUID) (map[Status]int64, error) {
	return s.repo.CountByStatus(ctx, channelIDs, productID)
}

func (s *Service) hydrate(ctx context.Context, rows []Review, total int64) ([]Review, int64, error) {
	if len(rows) == 0 {
		return rows, total, nil
	}
	ids := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	answers, err := s.repo.ListAnswersByReviewIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	media, err := s.repo.ListMediaByReviewIDs(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	for i := range rows {
		rows[i].Answers = answers[rows[i].ID]
		rows[i].Media = media[rows[i].ID]
	}
	return rows, total, nil
}

func (s *Service) ApproveReview(ctx context.Context, id, moderatorID uuid.UUID, note string) (Review, error) {
	rev, err := s.repo.SetReviewStatus(ctx, id, StatusApproved, moderatorID, note)
	if err != nil {
		return Review{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &Approved{
			ReviewID:    rev.ID,
			ProductID:   rev.ProductID,
			ChannelID:   rev.ChannelID,
			ModeratorID: moderatorID,
		})
	}
	return rev, nil
}

func (s *Service) RejectReview(ctx context.Context, id, moderatorID uuid.UUID, note string) (Review, error) {
	rev, err := s.repo.SetReviewStatus(ctx, id, StatusRejected, moderatorID, note)
	if err != nil {
		return Review{}, err
	}
	if s.bus != nil {
		s.bus.Publish(ctx, &Rejected{
			ReviewID:    rev.ID,
			ProductID:   rev.ProductID,
			ChannelID:   rev.ChannelID,
			ModeratorID: moderatorID,
			Note:        note,
		})
	}
	return rev, nil
}

// ---- AI: moderation, summary, reply (all suggestion-only + config-gated) ----

// Runs AI moderation and stores the verdict, which NEVER changes status - it's a hint for the human moderator
// No-op when AI is unavailable or the verdict is unparseable - Called async off review.Submitted and from the re-check mutation
func (s *Service) ClassifyReview(ctx context.Context, reviewID uuid.UUID) (Review, error) {
	if !s.aiReady(ctx) {
		return Review{}, ai.ErrNotConfigured
	}
	rev, err := s.GetReviewWithDetails(ctx, reviewID)
	if err != nil {
		return Review{}, err
	}
	// Active categories steer the classifier
	cats, err := s.repo.ListActiveCategories(ctx)
	if err != nil {
		return Review{}, err
	}
	opts := make([]ai.ReviewCategoryOption, len(cats))
	for i, c := range cats {
		opts[i] = ai.ReviewCategoryOption{Slug: c.Slug, Name: c.Name, Description: c.Description}
	}
	// Active moderation tags steer concern flagging (profanity, spam, …)
	tags, err := s.repo.ListActiveModerationTags(ctx)
	if err != nil {
		return Review{}, err
	}
	tagOpts := make([]ai.ModerationTagOption, len(tags))
	for i, t := range tags {
		tagOpts[i] = ai.ModerationTagOption{Slug: t.Slug, Name: t.Name, Description: t.Description}
	}
	verdict, err := s.ai.ModerateReview(ctx, rev.Title, reviewBodyText(rev), opts, tagOpts)
	if err != nil {
		return Review{}, err
	}
	if !verdict.Parsed {
		return rev, nil // unparseable - leave as-is
	}
	// Coarse label from the flags - no concerns → "genuine", any flag → "flagged"
	label := "genuine"
	if len(verdict.Tags) > 0 {
		label = "flagged"
	}
	return s.repo.SetReviewModeration(ctx, reviewID, label, verdict.Reason, verdict.Categories, verdict.Tags)
}

// SetMerchantReply stores the merchant's reply (empty body clears it), shown on the storefront
func (s *Service) SetMerchantReply(ctx context.Context, reviewID uuid.UUID, body string, adminID uuid.UUID) (Review, error) {
	return s.repo.SetMerchantReply(ctx, reviewID, body, adminID)
}

// GetProductSummary returns the cached AI summary ("" when none) - Pure read, no AI call
func (s *Service) GetProductSummary(ctx context.Context, productID uuid.UUID) (string, error) {
	// AI_ENABLED=false hides even cached summaries - no "AI summary" branding on an AI-free deploy
	if s.ai != nil && !s.ai.Enabled() {
		return "", nil
	}
	sum, ok, err := s.repo.GetProductSummary(ctx, productID)
	if err != nil || !ok {
		return "", err
	}
	return sum.Summary, nil
}

// MarkSummaryStale flags a summary for regeneration (on approve/reject, so it tracks the live set)
func (s *Service) MarkSummaryStale(ctx context.Context, productID uuid.UUID) error {
	return s.repo.MarkSummaryStale(ctx, productID)
}

// Regenerates and caches a product's summary, where productName is an optional prompt aid the scheduler passes as ""
// Under minReviewsForSummary approved reviews it gets no summary, marked fresh so the job stops rescanning
func (s *Service) GenerateProductSummary(ctx context.Context, productID uuid.UUID, productName string) (string, error) {
	if !s.aiReady(ctx) {
		return "", ai.ErrNotConfigured
	}
	rows, _, err := s.ListReviewsForProduct(ctx, productID, 50, 0)
	if err != nil {
		return "", err
	}
	if len(rows) < minReviewsForSummary {
		_ = s.repo.MarkSummaryFresh(ctx, productID)
		return "", nil
	}
	texts := make([]string, 0, len(rows))
	for _, rv := range rows {
		if t := reviewText(rv); t != "" {
			texts = append(texts, t)
		}
	}
	summary, err := s.ai.SummarizeReviews(ctx, productName, texts)
	if err != nil {
		return "", err
	}
	if err := s.repo.UpsertProductSummary(ctx, productID, summary, len(rows)); err != nil {
		return "", err
	}
	return summary, nil
}

// Regenerates up to `batch` stale summaries for the scheduler job - no-op when AI is unavailable, and best-effort per product
func (s *Service) RefreshDueSummaries(ctx context.Context, batch int32) (int, error) {
	if !s.aiReady(ctx) {
		return 0, nil
	}
	ids, err := s.repo.ListStaleSummaryIDs(ctx, batch)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, pid := range ids {
		if _, err := s.GenerateProductSummary(ctx, pid, ""); err != nil {
			log.Printf("[review] refresh summary %s: %v", pid, err)
			continue
		}
		n++
	}
	return n, nil
}

// A review's full text - title, body and free-text answers
func reviewText(rv Review) string {
	return strings.TrimSpace(rv.Title + " " + reviewBodyText(rv))
}

func reviewBodyText(rv Review) string {
	parts := []string{rv.Body}
	for _, a := range rv.Answers {
		if strings.TrimSpace(a.TextValue) != "" {
			parts = append(parts, a.TextValue)
		}
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

// GetAggregates and GetAggregatesByProductIDs live in remote.go (remote-first, falling back to s.repo) - The batch form
// omits products with no data, so callers treat a missing key as the empty Aggregates value

// --- Votes ---

// VoteReview records or clears a vote - value 1 (helpful), -1 (not), 0 (clear) voter carries exactly one identity; anonymous is rejected
// when the policy locks voting to signed-in - Returns the new totals so the caller skips a follow-up read
func (s *Service) VoteReview(ctx context.Context, voter Voter, reviewID uuid.UUID, value int16) (VoteCounts, error) {
	if value < -1 || value > 1 {
		return VoteCounts{}, ErrInvalidVote
	}
	if !voter.IsValid() {
		return VoteCounts{}, ErrInvalidVoter
	}
	if voter.IsAnonymous() && s.ReadPolicy(ctx).RequireAuthenticationToVote {
		return VoteCounts{}, ErrVoteAuthRequired
	}
	if value == 0 {
		if err := s.repo.DeleteVote(ctx, reviewID, voter); err != nil {
			return VoteCounts{}, err
		}
	} else {
		if err := s.repo.UpsertVote(ctx, reviewID, voter, value); err != nil {
			return VoteCounts{}, err
		}
	}
	counts, err := s.repo.GetVoteCountsByReviewIDs(ctx, []uuid.UUID{reviewID})
	if err != nil {
		return VoteCounts{}, err
	}
	return counts[reviewID], nil
}

func (s *Service) GetVoteCountsByReviewIDs(ctx context.Context, reviewIDs []uuid.UUID) (map[uuid.UUID]VoteCounts, error) {
	return s.repo.GetVoteCountsByReviewIDs(ctx, reviewIDs)
}

// GetMyVotesByReviewIDs returns the caller's own vote per review id (missing =
// not voted) - Anon by voter_token, signed-in by customer_id
func (s *Service) GetMyVotesByReviewIDs(ctx context.Context, voter Voter, reviewIDs []uuid.UUID) (map[uuid.UUID]int16, error) {
	if !voter.IsValid() {
		return map[uuid.UUID]int16{}, nil
	}
	return s.repo.GetMyVotesByReviewIDs(ctx, voter, reviewIDs)
}

// --- Eligibility ---

// Eligibility answers "can this customer review this product now?" - a structured outcome for the UI
type Eligibility struct {
	CanReview        bool
	Reason           EligibilityReason
	ExistingReviewID *uuid.UUID
}

type EligibilityReason string

const (
	EligibilityNotAuthenticated EligibilityReason = "not_authenticated"
	EligibilityNotPurchased     EligibilityReason = "not_purchased"
	EligibilityAlreadyReviewed  EligibilityReason = "already_reviewed"
)

// CheckEligibility decides whether the caller can post a new review, reading the
// live policy knobs - Order: anon+auth-required → NOT_AUTHENTICATED; anon+auth-off
// → eligible; already reviewed → ALREADY_REVIEWED (+ id); verified-required+not
// purchased → NOT_PURCHASED; else eligible
func (s *Service) CheckEligibility(ctx context.Context, customerID, productID uuid.UUID) (Eligibility, error) {
	policy := s.ReadPolicy(ctx)

	if customerID == uuid.Nil {
		if policy.RequireAuthentication {
			return Eligibility{CanReview: false, Reason: EligibilityNotAuthenticated}, nil
		}
		// Guest-eligible - skip the purchase check (no customer id), storefront still routes through sign-in to save
		return Eligibility{CanReview: true}, nil
	}

	existing, err := s.repo.GetReviewByCustomerAndProduct(ctx, customerID, productID)
	if err == nil {
		id := existing.ID
		return Eligibility{
			CanReview:        false,
			Reason:           EligibilityAlreadyReviewed,
			ExistingReviewID: &id,
		}, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return Eligibility{}, err
	}

	if policy.RequireVerifiedPurchase {
		purchased, err := s.repo.HasCustomerPurchased(ctx, customerID, productID)
		if err != nil {
			return Eligibility{}, err
		}
		if !purchased {
			return Eligibility{CanReview: false, Reason: EligibilityNotPurchased}, nil
		}
	}
	return Eligibility{CanReview: true}, nil
}
