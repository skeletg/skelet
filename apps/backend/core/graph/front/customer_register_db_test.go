package front

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/graph/admin/model"
	"github.com/skeletg/skelet/apps/backend/core/internal/authstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/channel"
	"github.com/skeletg/skelet/apps/backend/core/internal/configstore"
	"github.com/skeletg/skelet/apps/backend/core/internal/consent"
	"github.com/skeletg/skelet/apps/backend/core/internal/customer"
	db "github.com/skeletg/skelet/apps/backend/core/internal/db/sqlc"
	"github.com/skeletg/skelet/apps/backend/core/internal/events"
	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/config"
	"github.com/skeletg/skelet/apps/backend/core/lib/email"
	"github.com/skeletg/skelet/apps/backend/core/lib/testdb"
	"github.com/skeletg/skelet/apps/backend/core/lib/txrunner"
)

type accountStores struct{}

func (accountStores) GetByID(_ context.Context, id uuid.UUID) (customer.StoreSnapshot, error) {
	return customer.StoreSnapshot{ID: id, PublicURL: "https://shop.invalid"}, nil
}

type accountFixture struct {
	r   *mutationResolver
	cfg *config.Service
	ctx context.Context
	ch  uuid.UUID
}

func newAccountFixture(t *testing.T) *accountFixture {
	t.Helper()
	conn := testdb.Require(t)
	q := db.New(conn)
	tokens := authstore.NewPostgresTokenBackend(q)
	refresh := auth.NewRefreshStore(authstore.NewPostgresRefreshBackend(q, "customer"), nil, "customer")
	cfg := config.NewService(configstore.New(q))
	svc := customer.NewService(customer.NewRepository(q, nil, txrunner.New(conn)), auth.NewVerificationStore(tokens),
		auth.NewPasswordResetStore(tokens), refresh, email.LogSender{}, accountStores{},
		consent.NewService(consent.NewRepository(q)), events.NewBus())
	svc.SetConfigService(cfg)

	var ch channel.Channel
	if err := conn.QueryRow(`INSERT INTO channels (slug, name, currency_code, default_locale)
		SELECT $1, $1, currency_code, default_locale FROM channels ORDER BY created_at LIMIT 1 RETURNING id`,
		"acct-"+uuid.NewString()[:8]).Scan(&ch.ID); err != nil {
		t.Skipf("no channel to clone: %v", err)
	}
	t.Cleanup(func() {
		svc.FlushMail()
		_, _ = conn.Exec(`DELETE FROM customers WHERE channel_id = $1`, ch.ID)
		_, _ = conn.Exec(`DELETE FROM channels WHERE id = $1`, ch.ID)
	})
	r := &mutationResolver{&Resolver{
		CustomerSvc: svc, CustomerRefreshStore: refresh, JWTSecret: "test-secret", JWTTTL: time.Minute,
		LoginThrottle: auth.NewLoginThrottle(nil, 3, time.Minute, time.Minute),
	}}
	return &accountFixture{r: r, cfg: cfg, ctx: channel.WithChannel(context.Background(), &ch), ch: ch.ID}
}

func (f *accountFixture) gate(t *testing.T, on bool) {
	t.Helper()
	if _, err := f.cfg.Set(f.ctx, "general/security/require_email_verification", strconv.FormatBool(on),
		config.Scope{Kind: config.ScopeChannel, ChannelID: &f.ch}); err != nil {
		t.Fatal(err)
	}
}

func (f *accountFixture) register(addr, password string) (*model.CustomerAuthPayload, error) {
	return f.r.CustomerRegister(f.ctx, model.CustomerRegisterInput{Email: addr, Password: password, FirstName: "A", LastName: "B"})
}

// With the gate on, a new address and a taken one must produce byte-for-byte the same answer
func TestRegisterAnswersATakenAddressLikeANewOne(t *testing.T) {
	f := newAccountFixture(t)
	f.gate(t, true)
	addr := "taken-" + uuid.NewString()[:8] + "@example.invalid"

	fresh, err := f.register(addr, "password123")
	if err != nil {
		t.Fatal(err)
	}
	taken, err := f.register(addr, "another-password")
	if err != nil {
		t.Fatalf("taken address = %v, want the same success-shaped answer", err)
	}
	if !reflect.DeepEqual(fresh, taken) {
		t.Errorf("new registration %+v and taken address %+v differ", fresh, taken)
	}
	if fresh.Customer != nil || fresh.AccessToken != nil || !fresh.VerificationRequired {
		t.Errorf("gated registration = %+v, want no session, no account and verificationRequired", fresh)
	}
}

// With the gate off a new account signs in, but a taken address must never hand out a session
func TestRegisterNeverSignsInOnATakenAddress(t *testing.T) {
	f := newAccountFixture(t)
	addr := "open-" + uuid.NewString()[:8] + "@example.invalid"

	fresh, err := f.register(addr, "password123")
	if err != nil || fresh.AccessToken == nil || fresh.Customer == nil {
		t.Fatalf("open registration = %+v (%v), want a session", fresh, err)
	}
	taken, err := f.register(addr, "password123")
	if err != nil {
		t.Fatal(err)
	}
	if taken.AccessToken != nil || taken.RefreshToken != nil || taken.Customer != nil || !taken.VerificationRequired {
		t.Errorf("taken address = %+v, want no session and no account", taken)
	}
}

// Only a wrong password counts toward the lockout - a correct one on an unverified account is not a guess
func TestLoginLockoutCountsOnlyWrongPasswords(t *testing.T) {
	f := newAccountFixture(t)
	addr := "lock-" + uuid.NewString()[:8] + "@example.invalid"
	if _, err := f.register(addr, "password123"); err != nil {
		t.Fatal(err)
	}
	f.gate(t, true)
	login := func(pw string) error {
		_, err := f.r.CustomerLogin(f.ctx, model.CustomerLoginInput{Email: addr, Password: pw})
		return err
	}
	for range 5 {
		if err := login("password123"); !errors.Is(err, customer.ErrEmailNotVerified) {
			t.Fatalf("unverified login = %v, want ErrEmailNotVerified", err)
		}
	}
	for range 3 {
		_ = login("wrong-password")
	}
	if err := login("password123"); !errors.Is(err, auth.ErrAccountLocked) {
		t.Errorf("after three wrong passwords = %v, want ErrAccountLocked", err)
	}
}
