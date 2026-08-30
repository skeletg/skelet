package front

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/skeletg/skelet/graph/front/model"
	"github.com/skeletg/skelet/internal/auth"
	"github.com/skeletg/skelet/internal/ptr"
	"github.com/skeletg/skelet/internal/webform"
)

// Rate limited, honeypot/CAPTCHA checked and fully re-validated server-side - the service recomputes conditional visibility so a client can't skip a required field by hiding it
// Field-level failures ride the result payload as ok=false plus errors, while spam / rate-limit / inactive-form surface as GraphQL errors
func (r *mutationResolver) SubmitWebForm(ctx context.Context, input model.SubmitWebFormInput) (*model.SubmitWebFormResult, error) {
	if err := r.checkRate(ctx, "webform_submit"); err != nil {
		return nil, err
	}
	formID, err := uuid.Parse(input.FormID)
	if err != nil {
		return nil, invalidID("form")
	}
	storeID := channelIDFromContext(ctx)
	if storeID == nil {
		return nil, storeRequired()
	}

	values, _ := input.Values.(map[string]any)
	if values == nil {
		values = map[string]any{}
	}
	files := make([]webform.SubmissionFile, 0, len(input.Files))
	for _, f := range input.Files {
		if f == nil {
			continue
		}
		sf := webform.SubmissionFile{FieldKey: f.FieldKey, ObjectKey: f.ObjectKey, Name: f.Name}
		if f.Size != nil {
			sf.Size = int64(*f.Size)
		}
		if f.MimeType != nil {
			sf.MimeType = *f.MimeType
		}
		files = append(files, sf)
	}

	in := webform.SubmitInput{
		FormID:       formID,
		ChannelID:    *storeID,
		Values:       values,
		Files:        files,
		Honeypot:     ptr.Deref(input.Honeypot),
		CaptchaToken: ptr.Deref(input.CaptchaToken),
		CustomerID:   auth.OptionalCustomerID(ctx),
		Meta:         webform.SubmissionMeta{Locale: r.activeLocale(ctx)},
	}
	if req, ok := auth.RequestFromContext(ctx); ok {
		in.RemoteIP = auth.ClientIP(req)
		in.Meta.IP = in.RemoteIP
		in.Meta.UA = req.UserAgent()
		in.Meta.Referer = req.Referer()
	}

	res, err := r.WebFormSvc.Submit(ctx, in)
	if err != nil {
		if se, ok := errors.AsType[*webform.SubmissionError](err); ok {
			out := &model.SubmitWebFormResult{Ok: false, Errors: make([]*model.WebFormFieldError, 0, len(se.Fields))}
			for _, fe := range se.Fields {
				out.Errors = append(out.Errors, &model.WebFormFieldError{Field: fe.Field, Message: fe.Message})
			}
			return out, nil
		}
		return nil, err
	}
	out := &model.SubmitWebFormResult{Ok: true, Errors: []*model.WebFormFieldError{}}
	if res.SuccessMessage != "" {
		out.SuccessMessage = &res.SuccessMessage
	}
	if res.RedirectURL != "" {
		out.RedirectURL = &res.RedirectURL
	}
	return out, nil
}

// The object key is server-built, never client-supplied - the client uploads straight to the URL then echoes the key back on submit, where the server Head-validates it
func (r *mutationResolver) PresignWebFormUpload(ctx context.Context, formID string, fieldKey string, filename string, contentType string, sizeBytes int32) (*model.WebFormUploadTicket, error) {
	if err := r.checkRate(ctx, "webform_upload"); err != nil {
		return nil, err
	}
	fid, err := uuid.Parse(formID)
	if err != nil {
		return nil, invalidID("form")
	}
	url, key, err := r.WebFormSvc.PresignUpload(ctx, fid, fieldKey, filename, contentType, int64(sizeBytes))
	if err != nil {
		return nil, err
	}
	return &model.WebFormUploadTicket{URL: url, ObjectKey: key}, nil
}
