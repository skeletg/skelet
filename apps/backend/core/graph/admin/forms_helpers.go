package admin

// Lives here rather than forms.resolvers.go - gqlgen evicts non-resolver funcs from generated resolver files

import (
	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/form"
)

func formToModel(f form.Form, submissions int) *model.SignupForm {
	var seg *string
	if f.TargetSegmentID != nil {
		s := f.TargetSegmentID.String()
		seg = &s
	}
	return &model.SignupForm{
		ID: f.ID.String(), ChannelID: f.ChannelID.String(), Slug: f.Slug, Name: f.Name, Status: f.Status,
		Type: f.Type, Heading: f.Heading, Body: f.Body, ButtonLabel: f.ButtonLabel, SuccessMessage: f.SuccessMessage,
		CollectName: f.CollectName, ConsentKind: f.ConsentKind, TargetSegmentID: seg, TriggerFlow: f.TriggerFlow,
		Display: &model.SignupFormDisplay{
			Trigger: orStr(f.Display.Trigger, "immediate"), DelaySeconds: int32(f.Display.DelaySeconds),
			ScrollPercent: int32(f.Display.ScrollPercent), FrequencyDays: int32(f.Display.FrequencyDays),
		},
		SubmissionCount: int32(submissions),
	}
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

func saveInputToForm(in model.SaveSignupFormInput, channelID uuid.UUID) form.SaveInput {
	var seg *uuid.UUID
	if in.TargetSegmentID != nil && *in.TargetSegmentID != "" {
		if id, err := uuid.Parse(*in.TargetSegmentID); err == nil {
			seg = &id
		}
	}
	return form.SaveInput{
		ChannelID: channelID, Slug: in.Slug, Name: in.Name, Status: in.Status, Type: in.Type,
		Heading: in.Heading, Body: in.Body, ButtonLabel: in.ButtonLabel, SuccessMessage: in.SuccessMessage,
		CollectName: in.CollectName, ConsentKind: in.ConsentKind, TargetSegmentID: seg, TriggerFlow: in.TriggerFlow,
		Display: form.DisplayConfig{
			Trigger: in.Display.Trigger, DelaySeconds: int(in.Display.DelaySeconds),
			ScrollPercent: int(in.Display.ScrollPercent), FrequencyDays: int(in.Display.FrequencyDays),
		},
	}
}
