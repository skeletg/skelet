package order

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/apperr"
)

// Stage is a plugin's step inside the statuses it lists - the status still drives payment and stock
type Stage struct {
	Key    string
	Label  string
	Within []string
	Plugin string
}

// SetStages takes the stages a channel offers, and every stage there is - an order keeps a stage, its owner and its
// label after its plugin is switched off on the channel, which only stops orders entering it
func (s *Service) SetStages(offered func(ctx context.Context, channelID uuid.UUID) []Stage, all func() []Stage) {
	s.stages, s.allStages = offered, all
}

func (s *Service) Stages(ctx context.Context, channelID uuid.UUID) []Stage {
	if s.stages == nil {
		return nil
	}
	return s.stages(ctx, channelID)
}

func findStage(stages []Stage, key string) (Stage, bool) {
	i := slices.IndexFunc(stages, func(st Stage) bool { return st.Key == key })
	if key == "" || i < 0 {
		return Stage{}, false
	}
	return stages[i], true
}

// knownStage is the stage a key names, wherever it's offered
func (s *Service) knownStage(key string) (Stage, bool) {
	if s.allStages == nil {
		return Stage{}, false
	}
	return findStage(s.allStages(), key)
}

// stageAfter is the stage an order keeps moving to status - its own when that stage sits there too
func (s *Service) stageAfter(o Order, status string) string {
	if st, ok := s.knownStage(o.Stage); ok && slices.Contains(st.Within, status) {
		return o.Stage
	}
	return ""
}

var errNestedMove = apperr.New("ORDER_MOVE_NESTED", apperr.ClassConflict, "an order can't be moved while a move of it is still being decided")

type decidingMoves struct{}

// transitioning asks order.transitioning about a move. A handler that moves the same order again from inside the
// answer is refused, or the two would ask each other forever
func (s *Service) transitioning(ctx context.Context, o Order, status, stage string) error {
	deciding, _ := ctx.Value(decidingMoves{}).([]uuid.UUID)
	if slices.Contains(deciding, o.ID) {
		return errNestedMove
	}
	if s.bus == nil {
		return nil
	}
	ctx = context.WithValue(ctx, decidingMoves{}, append(slices.Clip(deciding), o.ID))
	return s.bus.PublishSync(ctx, &OrderTransitioning{
		OrderID: o.ID, ChannelID: o.ChannelID, FromStatus: o.Status, ToStatus: status, FromStage: o.Stage, ToStage: stage,
	})
}

var errStageRaced = apperr.New("ORDER_STAGE_RACED", apperr.ClassConflict, "the order's stage changed while this was being decided - try again")

// settleStage clears a stage the new status doesn't hold, on the timeline too - the event comes back to publish after
// commit without an outbox. asked is the stage order.transitioning was asked about; another one under the lock means
// the answer was given about a different order, so the move is refused
func (s *Service) settleStage(ctx context.Context, r Repository, sqlTx *sql.Tx, moved Order, asked string) (*OrderStageChanged, error) {
	if moved.Stage != asked {
		return nil, errStageRaced
	}
	keep := s.stageAfter(moved, moved.Status)
	if keep == moved.Stage {
		return nil, nil
	}
	if err := r.SetOrderStage(ctx, moved.ID, keep); err != nil {
		return nil, err
	}
	if _, err := r.AddOrderEvent(ctx, OrderEvent{OrderID: moved.ID, Type: "stage_changed", Message: "Left the " + s.stageLabel(moved.Stage) + " stage"}); err != nil {
		return nil, fmt.Errorf("add order event: %w", err)
	}
	return s.stageChanged(ctx, sqlTx, moved, keep)
}

func (s *Service) stageChanged(ctx context.Context, sqlTx *sql.Tx, o Order, to string) (*OrderStageChanged, error) {
	e := &OrderStageChanged{OrderID: o.ID, ChannelID: o.ChannelID, Status: o.Status, FromStage: o.Stage, ToStage: to}
	if s.outbox == nil {
		return e, nil
	}
	if err := s.outbox.Enqueue(ctx, sqlTx, e); err != nil {
		return nil, fmt.Errorf("enqueue order.stage_changed: %w", err)
	}
	return nil, nil
}

func (s *Service) publishStageChange(ctx context.Context, e *OrderStageChanged) {
	if e != nil && s.bus != nil {
		s.bus.Publish(ctx, e)
	}
}

func (s *Service) stageLabel(key string) string {
	if st, ok := s.knownStage(key); ok {
		return st.Label
	}
	return key
}

func (s *Service) moveStatus(ctx context.Context, o Order, to string, write func(r Repository) (Order, error)) (Order, error) {
	if err := s.transitioning(ctx, o, to, s.stageAfter(o, to)); err != nil {
		return Order{}, err
	}
	var (
		moved  Order
		staged *OrderStageChanged
	)
	err := s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		var err error
		if moved, err = write(r); err != nil {
			return err
		}
		staged, err = s.settleStage(ctx, r, sqlTx, moved, o.Stage)
		return err
	})
	if err != nil {
		return Order{}, err
	}
	s.publishStageChange(ctx, staged)
	moved.Stage = s.stageAfter(moved, moved.Status)
	return moved, nil
}

// plugin is the plugin asking, which may only move its own stages, and empty for an admin
func (s *Service) SetStage(ctx context.Context, orderID uuid.UUID, key, plugin string) (Order, error) {
	o, err := s.repo.GetOrderByID(ctx, orderID)
	if err != nil {
		return Order{}, err
	}
	key = strings.TrimSpace(key)
	if key == o.Stage {
		return o, nil
	}
	notYours := apperr.New("ORDER_STAGE_NOT_YOURS", apperr.ClassForbidden, "a plugin moves orders into and out of its own stages only")
	if key != "" {
		st, ok := findStage(s.Stages(ctx, o.ChannelID), key)
		switch {
		case !ok:
			return Order{}, apperr.New("ORDER_STAGE_UNKNOWN", apperr.ClassValidation, fmt.Sprintf("no stage %q on this order's channel", key))
		case !slices.Contains(st.Within, o.Status):
			return Order{}, apperr.New("ORDER_STAGE_NOT_HERE", apperr.ClassConflict,
				fmt.Sprintf("%s isn't a stage of a %s order", st.Label, strings.ReplaceAll(o.Status, "_", " ")))
		case plugin != "" && st.Plugin != plugin:
			return Order{}, notYours
		}
	}
	if cur, ok := s.knownStage(o.Stage); plugin != "" && o.Stage != "" && (!ok || cur.Plugin != plugin) {
		return Order{}, notYours
	}
	if err := s.transitioning(ctx, o, o.Status, key); err != nil {
		return Order{}, err
	}
	msg := "Stage set to " + s.stageLabel(key)
	if key == "" {
		msg = "Left the " + s.stageLabel(o.Stage) + " stage"
	}
	var changed *OrderStageChanged
	err = s.tx.InTransaction(ctx, func(sqlTx *sql.Tx) error {
		r := s.repo.WithTx(sqlTx)
		if err := r.SetOrderStageFrom(ctx, o.ID, o.Status, o.Stage, key); err != nil {
			return err
		}
		if _, err := r.AddOrderEvent(ctx, OrderEvent{OrderID: o.ID, Type: "stage_changed", Message: msg}); err != nil {
			return fmt.Errorf("add order event: %w", err)
		}
		changed, err = s.stageChanged(ctx, sqlTx, o, key)
		return err
	})
	if err != nil {
		return Order{}, err
	}
	s.publishStageChange(ctx, changed)
	return s.repo.GetOrderByID(ctx, orderID)
}
