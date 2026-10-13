package customer

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// Rows across the schema that name a customer by id or by address, seeded so erasure and hard delete can be checked
// against every one of them
type footprint struct {
	f        *fixture
	ch       uuid.UUID
	product  uuid.UUID
	order    uuid.UUID
	guest    uuid.UUID
	elsewhre uuid.UUID
	audits   map[string]uuid.UUID
}

func (f *fixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, err := f.conn.Exec(q, args...); err != nil {
		t.Fatalf("seed: %v\n%s", err, q)
	}
}

func (f *fixture) seedFootprint(t *testing.T, ch uuid.UUID, c Customer, withOrder bool) *footprint {
	t.Helper()
	fp := &footprint{f: f, ch: ch, audits: map[string]uuid.UUID{}}
	suffix := uuid.NewString()[:8]
	var typeID uuid.UUID
	if err := f.conn.QueryRow(`INSERT INTO catalog_product_types (slug, name) VALUES ($1, $1) RETURNING id`, "gdpr-"+suffix).Scan(&typeID); err != nil {
		t.Fatal(err)
	}
	if err := f.conn.QueryRow(`INSERT INTO catalog_products (product_type_id, sku, url, kind) VALUES ($1, $2, $2, 'simple') RETURNING id`,
		typeID, "gdpr-"+suffix).Scan(&fp.product); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.conn.Exec(`DELETE FROM catalog_products WHERE id = $1`, fp.product)
		_, _ = f.conn.Exec(`DELETE FROM catalog_product_types WHERE id = $1`, typeID)
	})
	shout := "  " + strings.ToUpper(c.Email) + " "
	if err := f.conn.QueryRow(`INSERT INTO orders (channel_id, currency_code, email, customer_note, billing_first_name)
		SELECT id, currency_code, $2, 'ring me', 'Gina' FROM channels WHERE id = $1 RETURNING id`, ch, shout).Scan(&fp.guest); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.conn.Exec(`DELETE FROM orders WHERE id = $1`, fp.guest) })
	if withOrder {
		if err := f.conn.QueryRow(`INSERT INTO orders (channel_id, currency_code, customer_id, email)
			SELECT id, currency_code, $2, $3 FROM channels WHERE id = $1 RETURNING id`, ch, c.ID, c.Email).Scan(&fp.order); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _, _ = f.conn.Exec(`DELETE FROM orders WHERE id = $1`, fp.order) })
	}
	f.exec(t, `INSERT INTO product_questions (product_id, channel_id, customer_id, author_name, author_email, question)
		VALUES ($1, $2, $3, 'Ana Own', $4, 'mine?'), ($1, $2, NULL, 'Ana Guest', $5, 'as a guest?')`, fp.product, ch, c.ID, c.Email, shout)
	var signupForm, webForm uuid.UUID
	if err := f.conn.QueryRow(`INSERT INTO signup_forms (channel_id, slug, name) VALUES ($1, $2, 'n') RETURNING id`, ch, "s-"+suffix).Scan(&signupForm); err != nil {
		t.Fatal(err)
	}
	if err := f.conn.QueryRow(`INSERT INTO web_forms (channel_id, slug, name) VALUES ($1, $2, 'n') RETURNING id`, ch, "w-"+suffix).Scan(&webForm); err != nil {
		t.Fatal(err)
	}
	f.exec(t, `INSERT INTO signup_form_submissions (form_id, channel_id, email, customer_id) VALUES ($1, $2, $3, NULL), ($1, $2, 'x@y.invalid', $4)`, signupForm, ch, c.Email, c.ID)
	f.exec(t, `INSERT INTO web_form_submissions (form_id, channel_id, answers, customer_id) VALUES
		($1, $2, jsonb_build_object('email', $3::text), NULL), ($1, $2, '{"q":"a"}', $4)`, webForm, ch, strings.ToUpper(c.Email), c.ID)
	f.exec(t, `INSERT INTO download_grants (token, order_id, customer_id, email, product_id, download_id, title, url)
		VALUES ($1, $2, NULL, $3, $4, $4, 't', 'u')`, "dg-"+suffix, fp.guest, c.Email, fp.product)
	f.exec(t, `INSERT INTO gift_cards (code, currency_code, initial_balance, balance, recipient_email, message, customer_id, channel_id)
		SELECT $1, currency_code, 100, 100, $2, 'happy birthday Ana', NULL, id FROM channels WHERE id = $3`, "GCR-"+suffix, c.Email, ch)
	f.exec(t, `INSERT INTO gift_cards (code, currency_code, initial_balance, balance, recipient_email, message, customer_id, channel_id)
		SELECT $1, currency_code, 100, 100, 'friend@example.invalid', 'from Ana', $2, id FROM channels WHERE id = $3`, "GCP-"+suffix, c.ID, ch)
	t.Cleanup(func() {
		_, _ = f.conn.Exec(`DELETE FROM gift_cards WHERE code IN ($1, $2)`, "GCR-"+suffix, "GCP-"+suffix)
	})
	f.exec(t, `INSERT INTO customer_payment_methods (customer_id, provider_slug, token, brand, last4) VALUES ($1, 'stripe', 'pm_secret', 'visa', '4242')`, c.ID)
	f.exec(t, `INSERT INTO wishlist_shares (customer_id, token) VALUES ($1, $2)`, c.ID, "ws-"+suffix)
	f.exec(t, `INSERT INTO back_in_stock_requests (channel_id, product_id, sku, email, customer_id) VALUES ($1, $2, 'sku', $3, NULL)`, ch, fp.product, shout)
	fp.elsewhre = f.newChannel(t)
	f.exec(t, `INSERT INTO back_in_stock_requests (channel_id, product_id, sku, email) VALUES ($1, $2, 'sku', $3)`, fp.elsewhre, fp.product, c.Email)
	for key, detail := range map[string]string{
		"email": `{"input":{"email":"` + strings.ToUpper(c.Email) + `"}}`,
		"id":    `{"customerId":"` + c.ID.String() + `"}`,
		"guest": `{"orderId":"` + fp.guest.String() + `"}`,
		"near":  `{"email":"` + strings.Replace(c.Email, "_", "Z", 1) + `"}`,
	} {
		var id uuid.UUID
		if err := f.conn.QueryRow(`INSERT INTO audit_log (action, detail) VALUES ('test', $1::jsonb) RETURNING id`, detail).Scan(&id); err != nil {
			t.Fatal(err)
		}
		fp.audits[key] = id
		t.Cleanup(func() { _, _ = f.conn.Exec(`DELETE FROM audit_log WHERE id = $1`, id) })
	}
	return fp
}

func (fp *footprint) count(t *testing.T, q string, args ...any) int {
	t.Helper()
	var n int
	if err := fp.f.conn.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatalf("%v\n%s", err, q)
	}
	return n
}

// Every seeded row naming the customer is gone or blank, and the rows of other people and other stores are not
func (fp *footprint) assertScrubbed(t *testing.T, c Customer) {
	t.Helper()
	addr := strings.ToLower(c.Email)
	for what, n := range map[string]int{
		"guest order still carrying the address":   fp.count(t, `SELECT count(*) FROM orders WHERE id = $1 AND (lower(email) LIKE '%'||$2||'%' OR customer_note <> '' OR billing_first_name <> 'Redacted')`, fp.guest, addr),
		"product questions with a name or address": fp.count(t, `SELECT count(*) FROM product_questions WHERE product_id = $1 AND (author_email <> '' OR author_name <> 'Redacted')`, fp.product),
		"signup submissions":                       fp.count(t, `SELECT count(*) FROM signup_form_submissions WHERE channel_id = $1`, fp.ch),
		"web form submissions":                     fp.count(t, `SELECT count(*) FROM web_form_submissions WHERE channel_id = $1`, fp.ch),
		"download grants with the address":         fp.count(t, `SELECT count(*) FROM download_grants WHERE product_id = $1 AND email <> ''`, fp.product),
		"gift cards with the address or a note":    fp.count(t, `SELECT count(*) FROM gift_cards WHERE channel_id = $1 AND (lower(recipient_email) = $2 OR message <> '')`, fp.ch, addr),
		"saved payment methods":                    fp.count(t, `SELECT count(*) FROM customer_payment_methods WHERE customer_id = $1`, c.ID),
		"wishlist shares":                          fp.count(t, `SELECT count(*) FROM wishlist_shares WHERE customer_id = $1`, c.ID),
		"back-in-stock on the customer's channel":  fp.count(t, `SELECT count(*) FROM back_in_stock_requests WHERE channel_id = $1`, fp.ch),
	} {
		if n != 0 {
			t.Errorf("%d %s survived", n, what)
		}
	}
	if n := fp.count(t, `SELECT count(*) FROM gift_cards WHERE channel_id = $1 AND recipient_email = 'friend@example.invalid'`, fp.ch); n != 1 {
		t.Error("the recipient address on a card the customer bought for someone else was erased - it isn't the customer's")
	}
	if n := fp.count(t, `SELECT count(*) FROM back_in_stock_requests WHERE channel_id = $1`, fp.elsewhre); n != 1 {
		t.Error("another channel's back-in-stock request for the same address was deleted - it belongs to another account")
	}
	for _, key := range []string{"email", "id", "guest"} {
		if n := fp.count(t, `SELECT count(*) FROM audit_log WHERE id = $1 AND detail ? 'redacted'`, fp.audits[key]); n != 1 {
			t.Errorf("audit row naming the customer by %s was not redacted", key)
		}
	}
	if n := fp.count(t, `SELECT count(*) FROM audit_log WHERE id = $1 AND detail ? 'redacted'`, fp.audits["near"]); n != 0 {
		t.Error("an audit row for a different address was redacted - the _ in the address matched as a wildcard")
	}
}

func underscoredEmail() string { return "cust_" + uuid.NewString()[:8] + "@example.invalid" }

func TestErasureScrubsEveryRowThatNamesTheCustomer(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, underscoredEmail(), "password123")
	fp := f.seedFootprint(t, ch, c, true)
	link := f.resetLink(t, c.ID)

	if err := f.svc.AnonymizeInPlace(f.ctx, c.ID); err != nil {
		t.Fatalf("anonymize: %v", err)
	}
	fp.assertScrubbed(t, c)
	if err := f.svc.ResetPassword(f.ctx, ch, link, "brand-new-password"); !errors.Is(err, ErrResetLinkInvalid) {
		t.Errorf("a reset link minted before erasure = %v, want revoked", err)
	}
	if n := fp.count(t, `SELECT count(*) FROM orders WHERE id = $1 AND email LIKE 'redacted-%'`, fp.order); n != 1 {
		t.Error("the customer's own order kept its address")
	}
}

func TestHardDeleteLeavesNoMoreBehindThanErasure(t *testing.T) {
	f := newFixture(t)
	ch := f.newChannel(t)
	c := f.register(t, ch, underscoredEmail(), "password123")
	fp := f.seedFootprint(t, ch, c, false)

	if err := f.svc.Delete(f.ctx, c.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	fp.assertScrubbed(t, c)
}
