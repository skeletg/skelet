package admin

import (
	"encoding/json"

	"github.com/skeletg/skelet/graph/admin/model"
	"github.com/skeletg/skelet/internal/webform"
)

// decodeJSON round-trips a GraphQL JSON scalar (decoded as `any`) into a typed destination. Used to turn the
// dashboard-authored fields/settings JSON into the domain structs the service validates
func decodeJSON(v any, dst any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}

func saveWebFormInput(in model.SaveWebFormInput) (webform.SaveInput, error) {
	out := webform.SaveInput{Name: in.Name, Status: webform.Status(in.Status)}
	if in.Slug != nil {
		out.Slug = *in.Slug
	}
	if err := decodeJSON(in.Fields, &out.Fields); err != nil {
		return out, errInvalidInput("fields", err)
	}
	if err := decodeJSON(in.Settings, &out.Settings); err != nil {
		return out, errInvalidInput("settings", err)
	}
	return out, nil
}

func webFormToModel(f webform.Form, count int64) *model.WebForm {
	var fields any = f.Fields
	if f.Fields == nil {
		fields = []webform.Field{} // JSON! must not be null
	}
	return &model.WebForm{
		ID:              f.ID.String(),
		ChannelID:       f.ChannelID.String(),
		Slug:            f.Slug,
		Name:            f.Name,
		Status:          string(f.Status),
		Fields:          fields,
		Settings:        f.Settings,
		SubmissionCount: int32(count),
		DeletedAt:       f.DeletedAt,
		CreatedAt:       f.CreatedAt,
		UpdatedAt:       f.UpdatedAt,
	}
}

func webFormSubmissionToModel(s webform.Submission) *model.WebFormSubmission {
	var answers any = s.Answers
	if s.Answers == nil {
		answers = map[string]any{}
	}
	var files any = s.Files
	if s.Files == nil {
		files = []webform.SubmissionFile{}
	}
	m := &model.WebFormSubmission{
		ID:        s.ID.String(),
		FormID:    s.FormID.String(),
		Answers:   answers,
		Files:     files,
		Meta:      s.Meta,
		CreatedAt: s.CreatedAt,
	}
	if s.CustomerID != nil {
		cid := s.CustomerID.String()
		m.CustomerID = &cid
	}
	return m
}
