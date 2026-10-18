// Package emailtemplate is the email builder: admin-editable emails of typed blocks, the reusable blocks they reference and the library they are saved in
// It renders to the same (subject, html, text, error) shape core's compiled templates answer, so a flow's mail and core's fallback are interchangeable
package emailtemplate

import (
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/email"
)

const (
	KeyOrderConfirmation  = "order_confirmation"
	KeyOrderShipment      = "order_shipment"
	KeyOrderCancellation  = "order_cancellation"
	KeyOrderRefund        = "order_refund"
	KeyVerification       = "customer_verification"
	KeyPasswordReset      = "password_reset"
	KeySignupConfirmation = "signup_confirmation"
	KeyAccountExists      = "account_exists"
	KeyCartOffer          = "cart_offer"
	KeyHeader             = "_header"
	KeyFooter             = "_footer"
)

// Editable keys in display order - the layout keys come last, and the admin UI groups them separately
var CanonicalKeys = []string{
	KeyOrderConfirmation, KeyOrderShipment, KeyOrderCancellation, KeyOrderRefund,
	KeyVerification, KeyPasswordReset, KeySignupConfirmation, KeyAccountExists, KeyCartOffer, KeyHeader, KeyFooter,
}

// Only the fields relevant to its Type are populated - extra fields are ignored on decode, and per-type validation happens at save
type Block struct {
	Type    string `json:"type"`
	Text    string `json:"text,omitempty"`    // heading
	Content string `json:"content,omitempty"` // text (multi-line)
	Label   string `json:"label,omitempty"`   // button
	URL     string `json:"url,omitempty"`     // button, image
	Alt     string `json:"alt,omitempty"`     // image
	Size    int    `json:"size,omitempty"`    // spacer (px)
	Slug    string `json:"slug,omitempty"`    // ref → email_blocks

	// A custom-block instance's field values, where Type names an email_block_types.key rather than a built-in type
	Fields map[string]string `json:"fields,omitempty"`

	Style *BlockStyle `json:"style,omitempty"`
	// Populated only for the "row" container - depth-1, so a column may not contain another row
	Columns []Column `json:"columns,omitempty"`

	// Gates the block on the recipient's merge vars - "show the VIP banner if customer.group is vip" - evaluated at render off the same vars the text interpolates, with no extra data source
	VisibleIf *BlockCondition `json:"visibleIf,omitempty"`
	Products  *ProductBlock   `json:"products,omitempty"`
	Social    []SocialLink    `json:"social,omitempty"`
	// The "video" block's thumbnail - email can't embed video, so it links out
	Poster   string `json:"poster,omitempty"`
	Deadline string `json:"deadline,omitempty"`
}

type SocialLink struct {
	Platform string `json:"platform"` // facebook | instagram | x | youtube | linkedin | tiktok | ...
	URL      string `json:"url"`
}

type BlockCondition struct {
	Field  string `json:"field"`
	Op     string `json:"op"` // exists | eq | ne | contains
	Value  string `json:"value,omitempty"`
	Negate bool   `json:"negate,omitempty"`
}

func (c *BlockCondition) eval(vars map[string]string) bool {
	if c == nil {
		return true
	}
	got := strings.TrimSpace(vars[c.Field])
	var ok bool
	switch c.Op {
	case "exists":
		ok = got != ""
	case "eq":
		ok = strings.EqualFold(got, c.Value)
	case "ne":
		ok = !strings.EqualFold(got, c.Value)
	case "contains":
		ok = got != "" && strings.Contains(strings.ToLower(got), strings.ToLower(c.Value))
	default:
		ok = true // unknown op → don't hide
	}
	if c.Negate {
		return !ok
	}
	return ok
}

type ProductBlock struct {
	Mode         string   `json:"mode"` // skus | collection
	SKUs         []string `json:"skus,omitempty"`
	CollectionID string   `json:"collectionId,omitempty"`
	Limit        int      `json:"limit,omitempty"` // collection cap (default 3)
	Columns      int      `json:"columns,omitempty"`
}

// Empty fields fall back to the email theme, then the built-in defaults - colors are validated at save and escaped at render, since they inline into style="" attributes
type BlockStyle struct {
	Align           string `json:"align,omitempty"`           // left | center | right
	Color           string `json:"color,omitempty"`           // text color
	FontSize        int    `json:"fontSize,omitempty"`        // px
	PaddingY        int    `json:"paddingY,omitempty"`        // px (top+bottom)
	PaddingX        int    `json:"paddingX,omitempty"`        // px (left+right)
	Background      string `json:"background,omitempty"`      // block background
	ButtonColor     string `json:"buttonColor,omitempty"`     // button background
	ButtonTextColor string `json:"buttonTextColor,omitempty"` // button label color
	Radius          int    `json:"radius,omitempty"`          // button corner px
	Width           int    `json:"width,omitempty"`           // image width %
	HideMobile      bool   `json:"hideMobile,omitempty"`
}

type Column struct {
	Width  int     `json:"width,omitempty"`
	Blocks []Block `json:"blocks,omitempty"`
}

type EmailStyles struct {
	Background   string `json:"background,omitempty"`
	ContentWidth int    `json:"contentWidth,omitempty"` // px
	FontFamily   string `json:"fontFamily,omitempty"`
	LinkColor    string `json:"linkColor,omitempty"`
	TextColor    string `json:"textColor,omitempty"`
}

type Template struct {
	Preheader string // inbox-preview snippet (only carried on slot emails today)
	Key       string
	ChannelID *uuid.UUID
	Subject   string
	Blocks    []Block
	Styles    EmailStyles
}

// The unified reusable block, per channel - an optional field schema parameterizes instances, and the body is either composed Blocks or admin-authored HTMLTemplate, sanitized at render
// With no fields and no HTML template it's a plain shared snippet; with them it's a reusable block type in its own right - Slug is its reference key
type Snippet struct {
	ChannelID    uuid.UUID
	Slug         string
	Name         string
	Icon         string
	Fields       []FieldDef
	Blocks       []Block // composed body (used when HTMLTemplate is empty)
	HTMLTemplate string  // advanced body; admin-trusted, sanitized at render
	TextTemplate string  // optional plain-text body
	// Sanitized and injected as a <style> at render, so classes and media queries work - empty means none
	CSS string
	// Renders the HTML body through Liquid - conditionals, loops, filters - instead of plain {{tag}} interpolation
	Scripted bool
}

// Instances supply values for these, escaped and interpolated into the body at render
type FieldDef struct {
	Key      string   `json:"key"`
	Label    string   `json:"label"`
	Kind     string   `json:"kind"` // text|textarea|url|color|number|date|image|select|boolean
	Required bool     `json:"required,omitempty"`
	Default  string   `json:"default,omitempty"`
	Options  []string `json:"options,omitempty"` // for kind=select
}

// Scalar money values arrive pre-formatted in Vars - Lines and ShipTo are populated only for order emails
type renderCtx struct {
	Vars      map[string]string
	Currency  string // ISO code, for the items/totals data blocks
	Lines     []email.OrderEmailLine
	ShipTo    []string
	Theme     EmailStyles // email-level look; blocks fall back to it for colors/fonts
	ChannelID uuid.UUID   // resolves refs + custom block types per channel
	refStack  []string    // chain of ref slugs being expanded (nesting cycle/depth guard)
}

const maxRefDepth = 8

// Same shape as core's compiled renderer, kept local so this package owns its formatting
func money(cents int64, currency string) string {
	return fmt.Sprintf("%.2f %s", float64(cents)/100, currency)
}

// Unknown tokens resolve to "" and literal text passes through - the caller HTML-escapes the result
func interpolate(s string, vars map[string]string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "{{")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		rest := s[i+2:]
		before, after, ok := strings.Cut(rest, "}}")
		if !ok {
			b.WriteString("{{")
			b.WriteString(rest)
			return b.String()
		}
		b.WriteString(resolveToken(strings.TrimSpace(before), vars))
		s = after
	}
}

// Supports a fallback so personalization can't produce the classic "Hi ,"
// {{x}} is the value or "", while {{x | default: "there"}} and the shorthand {{x | there}} both fall back
func resolveToken(tok string, vars map[string]string) string {
	key, fallback := tok, ""
	if before, after, ok := strings.Cut(tok, "|"); ok {
		key = strings.TrimSpace(before)
		fb := strings.TrimSpace(after)
		fb = strings.TrimSpace(strings.TrimPrefix(fb, "default:"))
		fallback = strings.Trim(fb, `"'`)
	}
	if v := vars[key]; v != "" {
		return v
	}
	return fallback
}
