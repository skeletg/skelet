package email

import (
	"bytes"
	"fmt"
	"html/template"
	ttemplate "text/template"
)

type VerificationEmailData struct {
	FirstName string
	Link      string // full URL the customer clicks to verify
	StoreName string
}

type PasswordResetEmailData struct {
	FirstName string
	Link      string // full URL the customer clicks to reset
	StoreName string
	ExpiresIn string // human-readable, e.g. "30 minutes"
}

// A signup-form subscriber confirming the address before any marketing mail goes to it
type SignupConfirmationEmailData struct {
	FirstName string
	Link      string
	StoreName string
	ExpiresIn string
}

const (
	subjectVerification       = "Verify your email address"
	subjectPasswordReset      = "Reset your password"
	subjectSignupConfirmation = "Confirm your subscription"
)

// Inline templates, kept plain on purpose - real branding lives in a frontend email builder or a provider's template system, and the goal here is raw text that reads with or without HTML rendering

var verificationHTML = template.Must(template.New("verify_html").Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;">
<p>Hi {{.FirstName}},</p>
<p>Welcome to {{.StoreName}}. Please confirm your email address by clicking the link below:</p>
<p><a href="{{.Link}}">Verify my email</a></p>
<p>If you didn't sign up, you can safely ignore this email.</p>
</body></html>`))

var verificationText = ttemplate.Must(ttemplate.New("verify_text").Parse(
	`Hi {{.FirstName}},

Welcome to {{.StoreName}}. Please confirm your email address by opening this link:

{{.Link}}

If you didn't sign up, you can safely ignore this email.
`))

var resetHTML = template.Must(template.New("reset_html").Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;">
<p>Hi {{.FirstName}},</p>
<p>We received a request to reset your {{.StoreName}} password. Click the link below to choose a new one:</p>
<p><a href="{{.Link}}">Reset my password</a></p>
<p>This link expires in {{.ExpiresIn}}. If you didn't request a reset, you can safely ignore this email - your password won't change.</p>
</body></html>`))

var resetText = ttemplate.Must(ttemplate.New("reset_text").Parse(
	`Hi {{.FirstName}},

We received a request to reset your {{.StoreName}} password. Open this link to choose a new one:

{{.Link}}

This link expires in {{.ExpiresIn}}. If you didn't request a reset, you can safely ignore this email - your password won't change.
`))

var signupConfirmHTML = template.Must(template.New("signup_confirm_html").Parse(`<!DOCTYPE html>
<html><body style="font-family:sans-serif;">
<p>Hi {{.FirstName}},</p>
<p>Please confirm you'd like to receive emails from {{.StoreName}} by clicking the link below:</p>
<p><a href="{{.Link}}">Confirm my subscription</a></p>
<p>This link expires in {{.ExpiresIn}}. If you didn't sign up, ignore this email and you won't be subscribed.</p>
</body></html>`))

var signupConfirmText = ttemplate.Must(ttemplate.New("signup_confirm_text").Parse(
	`Hi {{.FirstName}},

Please confirm you'd like to receive emails from {{.StoreName}} by opening this link:

{{.Link}}

This link expires in {{.ExpiresIn}}. If you didn't sign up, ignore this email and you won't be subscribed.
`))

// Template parse errors are panics caught at package init through template.Must, so only runtime execution errors surface here
func RenderVerificationEmail(data VerificationEmailData) (subject, html, text string, err error) {
	var hb, tb bytes.Buffer
	if err := verificationHTML.Execute(&hb, data); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := verificationText.Execute(&tb, data); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return subjectVerification, hb.String(), tb.String(), nil
}

func RenderPasswordResetEmail(data PasswordResetEmailData) (subject, html, text string, err error) {
	var hb, tb bytes.Buffer
	if err := resetHTML.Execute(&hb, data); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := resetText.Execute(&tb, data); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return subjectPasswordReset, hb.String(), tb.String(), nil
}

func RenderSignupConfirmationEmail(data SignupConfirmationEmailData) (subject, html, text string, err error) {
	if data.FirstName == "" {
		data.FirstName = greetingFallback
	}
	var hb, tb bytes.Buffer
	if err := signupConfirmHTML.Execute(&hb, data); err != nil {
		return "", "", "", fmt.Errorf("render html: %w", err)
	}
	if err := signupConfirmText.Execute(&tb, data); err != nil {
		return "", "", "", fmt.Errorf("render text: %w", err)
	}
	return subjectSignupConfirmation, hb.String(), tb.String(), nil
}
