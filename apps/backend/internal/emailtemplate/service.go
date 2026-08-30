package emailtemplate

import (
	"context"
	"fmt"
	"html"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/internal/config"
	"github.com/skeletg/skelet/internal/email"
)

// Renders transactional emails from admin templates, falling back to the built-in internal/email renderers, and backs the admin CRUD, preview and test-send - it reads branding from config and uses a Sender for tests
type Service struct {
	repo    Repository
	config  *config.Service
	sender  email.Sender
	catalog CatalogReader // optional; powers the "products" block
}

func NewService(repo Repository, cfg *config.Service, sender email.Sender) *Service {
	return &Service{repo: repo, config: cfg, sender: sender}
}

func (s *Service) RenderOrderConfirmation(ctx context.Context, channelID uuid.UUID, d email.OrderConfirmationData) (string, string, string, error) {
	tmpl, ok, err := s.resolve(ctx, KeyOrderConfirmation, channelID)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return email.RenderOrderConfirmation(d)
	}
	return s.renderTemplate(ctx, channelID, tmpl, s.orderConfirmationCtx(ctx, channelID, d))
}

func (s *Service) RenderOrderShipment(ctx context.Context, channelID uuid.UUID, d email.OrderShipmentData) (string, string, string, error) {
	tmpl, ok, err := s.resolve(ctx, KeyOrderShipment, channelID)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return email.RenderOrderShipment(d)
	}
	return s.renderTemplate(ctx, channelID, tmpl, s.orderShipmentCtx(ctx, channelID, d))
}

func (s *Service) RenderOrderCancellation(ctx context.Context, channelID uuid.UUID, d email.OrderCancellationData) (string, string, string, error) {
	tmpl, ok, err := s.resolve(ctx, KeyOrderCancellation, channelID)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return email.RenderOrderCancellation(d)
	}
	return s.renderTemplate(ctx, channelID, tmpl, s.orderCancellationCtx(ctx, channelID, d))
}

func (s *Service) RenderOrderRefund(ctx context.Context, channelID uuid.UUID, d email.OrderRefundData) (string, string, string, error) {
	tmpl, ok, err := s.resolve(ctx, KeyOrderRefund, channelID)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return email.RenderOrderRefund(d)
	}
	return s.renderTemplate(ctx, channelID, tmpl, s.orderRefundCtx(ctx, channelID, d))
}

func (s *Service) RenderVerificationEmail(ctx context.Context, channelID uuid.UUID, d email.VerificationEmailData) (string, string, string, error) {
	tmpl, ok, err := s.resolve(ctx, KeyVerification, channelID)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return email.RenderVerificationEmail(d)
	}
	return s.renderTemplate(ctx, channelID, tmpl, s.verificationCtx(ctx, channelID, d))
}

func (s *Service) RenderPasswordResetEmail(ctx context.Context, channelID uuid.UUID, d email.PasswordResetEmailData) (string, string, string, error) {
	tmpl, ok, err := s.resolve(ctx, KeyPasswordReset, channelID)
	if err != nil {
		return "", "", "", err
	}
	if !ok {
		return email.RenderPasswordResetEmail(d)
	}
	return s.renderTemplate(ctx, channelID, tmpl, s.passwordResetCtx(ctx, channelID, d))
}

// Renders standalone content - subject, blocks and theme - with a caller-supplied merge-tag map, for campaigns, which aren't tied to a transactional key or the header/footer layout
// `ref` blocks still resolve against the reusable library, while data blocks render empty, having no order context
func (s *Service) RenderContent(ctx context.Context, channelID uuid.UUID, subject, preheader string, blocks []Block, styles EmailStyles, vars map[string]string) (subjectOut, htmlOut, textOut string, err error) {
	rc := renderCtx{Vars: vars, Theme: styles, ChannelID: channelID}
	bodyH, bodyT, err := s.renderBlocks(ctx, blocks, rc)
	if err != nil {
		return "", "", "", err
	}
	return interpolate(subject, vars), wrapHTML(bodyH, styles, interpolate(preheader, vars)), bodyT, nil
}

// The same block and style validation as SaveTemplate, for campaign content, with refs allowed
func (s *Service) ValidateContent(blocks []Block, styles EmailStyles) error {
	if err := validateStyles(styles); err != nil {
		return err
	}
	for i, b := range blocks {
		if err := validateBlock(b, true); err != nil {
			return fmt.Errorf("block %d: %w", i+1, err)
		}
	}
	return nil
}

// The template for key, preferring a channel override over the global row - (false, nil) when neither exists, so the caller falls back to built-in
func (s *Service) resolve(ctx context.Context, key string, channelID uuid.UUID) (Template, bool, error) {
	// A Transactional email assigned to this slot wins, since the unified emails list is the primary source - it masquerades as a Template so the whole downstream render is identical
	// It falls through to the email_templates row and then built-in when no email is assigned, so unassigned slots render byte-identical to before
	if channelID != uuid.Nil && isTransactionalSlot(key) {
		if e, ok, err := s.repo.GetSlotEmail(ctx, channelID, key); err != nil {
			return Template{}, false, err
		} else if ok {
			cid := channelID
			return Template{Key: key, ChannelID: &cid, Subject: e.Subject, Preheader: e.Preheader, Blocks: e.Blocks, Styles: e.Styles}, true, nil
		}
	}
	if channelID != uuid.Nil {
		id := channelID
		if t, ok, err := s.repo.Get(ctx, key, &id); err != nil || ok {
			return t, ok, err
		}
	}
	return s.repo.Get(ctx, key, nil)
}

// Wraps the body blocks in the resolved header and footer plus base HTML chrome, themed by the body template's email styles, and interpolates the subject - the header and footer inherit the body's theme
func (s *Service) renderTemplate(ctx context.Context, channelID uuid.UUID, tmpl Template, rc renderCtx) (subject, htmlOut, textOut string, err error) {
	rc.Theme = tmpl.Styles
	rc.ChannelID = channelID
	headerH, headerT, err := s.renderLayout(ctx, channelID, KeyHeader, rc)
	if err != nil {
		return "", "", "", err
	}
	footerH, footerT, err := s.renderLayout(ctx, channelID, KeyFooter, rc)
	if err != nil {
		return "", "", "", err
	}
	bodyH, bodyT, err := s.renderBlocks(ctx, tmpl.Blocks, rc)
	if err != nil {
		return "", "", "", err
	}
	return interpolate(tmpl.Subject, rc.Vars), wrapHTML(headerH+bodyH+footerH, tmpl.Styles, interpolate(tmpl.Preheader, rc.Vars)), headerT + bodyT + footerT, nil
}

// Renders the _header or _footer template's blocks, empty when there are none
func (s *Service) renderLayout(ctx context.Context, channelID uuid.UUID, key string, rc renderCtx) (string, string, error) {
	tmpl, ok, err := s.resolve(ctx, key, channelID)
	if err != nil || !ok {
		return "", "", err
	}
	return s.renderBlocks(ctx, tmpl.Blocks, rc)
}

// Walks a block list, dispatching each - a built-in renderer, a per-channel custom block type, a nestable `ref` snippet expansion with a cycle and depth guard, or a `row` container
func (s *Service) renderBlocks(ctx context.Context, blocks []Block, rc renderCtx) (string, string, error) {
	var hb, tb strings.Builder
	for _, b := range blocks {
		// Conditional visibility - a block gated by VisibleIf renders only when its condition holds for this recipient's merge vars
		if b.VisibleIf != nil && !b.VisibleIf.eval(rc.Vars) {
			continue
		}
		var h, t string
		var err error
		switch b.Type {
		case "ref":
			// Per-use style override - the ref instance's Style wraps the component output
			h, t, err = s.renderRef(ctx, b, rc)
			h = boxHTML(b, h)
		case "row":
			h, t, err = s.renderRow(ctx, b, rc)
		case "products":
			h, t, err = s.renderProducts(ctx, b, rc)
		default:
			h, t, err = s.renderOne(ctx, b, rc)
		}
		if err != nil {
			return "", "", err
		}
		hb.WriteString(h)
		tb.WriteString(t)
	}
	return hb.String(), tb.String(), nil
}

// Expands a referenced reusable block inline, passing the instance's field values for parameterized blocks - snippets may nest refs, and a slug already on the stack or past maxRefDepth is skipped
func (s *Service) renderRef(ctx context.Context, b Block, rc renderCtx) (string, string, error) {
	return s.renderComponent(ctx, b.Slug, b.Fields, rc)
}

// Resolves a reusable block by slug per channel and renders it with the instance's field values - an HTML-body block through the safe HTML path, otherwise its composed blocks with the field values merged into the merge-tag vars so {{field}} placeholders interpolate - Depth and cycle guarded
func (s *Service) renderComponent(ctx context.Context, slug string, instanceFields map[string]string, rc renderCtx) (string, string, error) {
	if slug == "" || len(rc.refStack) >= maxRefDepth || containsStr(rc.refStack, slug) {
		return "", "", nil
	}
	snip, ok, err := s.repo.GetBlock(ctx, rc.ChannelID, slug)
	if err != nil || !ok {
		return "", "", err
	}
	vars := customVars(rc.Vars, instanceFields, snip.Fields)
	if strings.TrimSpace(snip.HTMLTemplate) != "" {
		var h, t string
		if snip.Scripted {
			// Liquid - conditionals, loops and filters over the email's data
			h, t = s.renderScriptedHTML(snip, vars, rc)
		} else {
			h, t = renderCustomHTML(snip.HTMLTemplate, snip.TextTemplate, vars)
		}
		// Custom CSS, sanitized, rides along as a <style> before the body
		if css := sanitizeCSS(snip.CSS); css != "" {
			h = "<style>" + css + "</style>" + h
		}
		return h, t, nil
	}
	next := rc
	next.Vars = vars
	next.refStack = append(append([]string{}, rc.refStack...), slug)
	return s.renderBlocks(ctx, snip.Blocks, next)
}

// Renders one block - a built-in type through its renderer, otherwise a reusable block referenced by slug, for instances whose Type names the slug - Renders nothing when absent
func (s *Service) renderOne(ctx context.Context, b Block, rc renderCtx) (string, string, error) {
	if f := blockHTML[b.Type]; f != nil {
		var tt string
		if g := blockText[b.Type]; g != nil {
			tt = g(b, rc)
		}
		return boxHTML(b, f(b, rc)), tt, nil
	}
	h, t, err := s.renderComponent(ctx, b.Type, b.Fields, rc)
	if err != nil {
		return "", "", err
	}
	return boxHTML(b, h), t, nil
}

// Lays a row's columns out as an email-safe table and stacks them in the plain-text part - columns hold leaf and custom blocks, no refs or rows
func (s *Service) renderRow(ctx context.Context, b Block, rc renderCtx) (string, string, error) {
	if len(b.Columns) == 0 {
		return "", "", nil
	}
	total := 0
	for _, c := range b.Columns {
		w := c.Width
		if w <= 0 {
			w = 1
		}
		total += w
	}
	var hb, tb strings.Builder
	hb.WriteString(`<table width="100%" cellpadding="0" cellspacing="0" style="border-collapse:collapse;"><tr>`)
	for _, c := range b.Columns {
		w := c.Width
		if w <= 0 {
			w = 1
		}
		fmt.Fprintf(&hb, `<td valign="top" style="width:%d%%;padding:0 6px;">`, w*100/total)
		for _, cb := range c.Blocks {
			if cb.Type == "ref" || cb.Type == "row" {
				continue
			}
			h, t, err := s.renderOne(ctx, cb, rc)
			if err != nil {
				return "", "", err
			}
			hb.WriteString(h)
			tb.WriteString(t)
		}
		hb.WriteString(`</td>`)
	}
	hb.WriteString(`</tr></table>`)
	return hb.String(), tb.String(), nil
}

func containsStr(ss []string, s string) bool {
	return slices.Contains(ss, s)
}

// Merges global merge tags, a custom type's field defaults and the instance's field values, instance winning, for template interpolation
func customVars(global, instance map[string]string, defs []FieldDef) map[string]string {
	v := make(map[string]string, len(global)+len(defs))
	maps.Copy(v, global)
	for _, d := range defs {
		v[d.Key] = d.Default
	}
	maps.Copy(v, instance)
	return v
}

func wrapHTML(inner string, theme EmailStyles, preheader string) string {
	bg := themeOr(theme.Background, "#f6f6f6")
	font := themeOr(theme.FontFamily, "Helvetica,Arial,sans-serif")
	text := themeOr(theme.TextColor, "#222")
	width := 600 // standard HTML-email content width
	if theme.ContentWidth >= 200 && theme.ContentWidth <= 1000 {
		width = theme.ContentWidth
	}
	linkStyle := `<style>@media only screen and (max-width:600px){.skl-hide-mobile{display:none!important}}</style>`
	if c := strings.TrimSpace(theme.LinkColor); c != "" {
		linkStyle += `<style>a{color:` + html.EscapeString(c) + `;}</style>`
	}
	// The hidden preheader, the inbox-preview snippet shown after the subject - display:none with zero size keeps it out of the visible body, and the trailing zero-width joiners stop clients pulling following body text into the preview
	pre := ""
	if p := strings.TrimSpace(preheader); p != "" {
		pre = `<div style="display:none;max-height:0;overflow:hidden;mso-hide:all;">` +
			html.EscapeString(p) + strings.Repeat("&#847;&zwnj;&nbsp;", 20) + `</div>`
	}
	return `<!DOCTYPE html><html><head>` + linkStyle + `</head><body style="margin:0;background:` + html.EscapeString(bg) +
		`;font-family:` + html.EscapeString(font) + `;color:` + html.EscapeString(text) + `;">` + pre +
		`<div style="max-width:` + strconv.Itoa(width) + `px;margin:24px auto;padding:24px;background:#fff;border-radius:6px;">` +
		inner + `</div></body></html>`
}

func themeOr(v, def string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return def
}

func (s *Service) commonVars(ctx context.Context, channelID uuid.UUID, storeName, firstName, storefrontURL string) map[string]string {
	return map[string]string{
		"store.name":         storeName,
		"store.logoUrl":      s.logoURL(ctx, channelID),
		"customer.firstName": firstNameOr(firstName),
		"storefront.url":     storefrontURL,
	}
}

func (s *Service) orderConfirmationCtx(ctx context.Context, channelID uuid.UUID, d email.OrderConfirmationData) renderCtx {
	v := s.commonVars(ctx, channelID, d.StoreName, d.FirstName, d.StorefrontURL)
	v["order.number"] = d.OrderNumber
	v["order.subtotal"] = money(d.Subtotal, d.Currency)
	v["order.shipping"] = money(d.Shipping, d.Currency)
	v["order.tax"] = money(d.Tax, d.Currency)
	v["order.total"] = money(d.Total, d.Currency)
	if d.Discount > 0 {
		v["order.discount"] = money(d.Discount, d.Currency)
	}
	return renderCtx{Vars: v, Currency: d.Currency, Lines: d.Lines, ShipTo: d.ShipTo}
}

func (s *Service) orderShipmentCtx(ctx context.Context, channelID uuid.UUID, d email.OrderShipmentData) renderCtx {
	v := s.commonVars(ctx, channelID, d.StoreName, d.FirstName, d.StorefrontURL)
	v["order.number"] = d.OrderNumber
	v["tracking.carrier"] = d.Carrier
	v["tracking.number"] = d.TrackingNumber
	return renderCtx{Vars: v, Currency: d.Currency, Lines: d.Lines}
}

func (s *Service) orderCancellationCtx(ctx context.Context, channelID uuid.UUID, d email.OrderCancellationData) renderCtx {
	v := s.commonVars(ctx, channelID, d.StoreName, d.FirstName, d.StorefrontURL)
	v["order.number"] = d.OrderNumber
	return renderCtx{Vars: v}
}

func (s *Service) orderRefundCtx(ctx context.Context, channelID uuid.UUID, d email.OrderRefundData) renderCtx {
	v := s.commonVars(ctx, channelID, d.StoreName, d.FirstName, d.StorefrontURL)
	v["order.number"] = d.OrderNumber
	v["refund.amount"] = money(d.RefundedAmount, d.Currency)
	return renderCtx{Vars: v, Currency: d.Currency}
}

func (s *Service) verificationCtx(ctx context.Context, channelID uuid.UUID, d email.VerificationEmailData) renderCtx {
	v := s.commonVars(ctx, channelID, d.StoreName, d.FirstName, "")
	v["verify.link"] = d.Link
	return renderCtx{Vars: v}
}

func (s *Service) passwordResetCtx(ctx context.Context, channelID uuid.UUID, d email.PasswordResetEmailData) renderCtx {
	v := s.commonVars(ctx, channelID, d.StoreName, d.FirstName, "")
	v["reset.link"] = d.Link
	v["reset.expiresIn"] = d.ExpiresIn
	return renderCtx{Vars: v}
}

func (s *Service) logoURL(ctx context.Context, channelID uuid.UUID) string {
	if s.config == nil {
		return ""
	}
	scope := config.DefaultScope
	if channelID != uuid.Nil {
		id := channelID
		scope = config.Scope{Kind: config.ScopeChannel, ChannelID: &id}
	}
	v, _ := s.config.GetString(ctx, "general/branding/logo_url", scope)
	return strings.TrimSpace(v)
}

func firstNameOr(name string) string {
	if strings.TrimSpace(name) == "" {
		return "there"
	}
	return name
}
