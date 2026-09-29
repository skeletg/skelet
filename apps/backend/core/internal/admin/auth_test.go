package admin

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"

	"github.com/skeletg/skelet/apps/backend/core/lib/auth"
	"github.com/skeletg/skelet/apps/backend/core/lib/secrets"
)

// In-memory stand-in for the Postgres repository, mirroring the conditional updates the service relies on
type memRepo struct {
	Repository
	users     map[uuid.UUID]*AdminUser
	lastStep  map[uuid.UUID]int64
	attempts  map[uuid.UUID]int
	now       func() time.Time
	roles     []CreateRoleInput
	deleted   []uuid.UUID
	redeemErr error
}

func newMemRepo(now func() time.Time) *memRepo {
	return &memRepo{users: map[uuid.UUID]*AdminUser{}, lastStep: map[uuid.UUID]int64{}, attempts: map[uuid.UUID]int{}, now: now}
}

func (m *memRepo) add(u AdminUser) *AdminUser {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	m.users[u.ID] = &u
	return &u
}

func (m *memRepo) Create(_ context.Context, in CreateAdminUserInput) (AdminUser, error) {
	for _, u := range m.users {
		if strings.EqualFold(u.Email, in.Email) {
			return AdminUser{}, ErrEmailTaken
		}
	}
	return *m.add(AdminUser{Email: in.Email, PasswordHash: in.PasswordHash, IsActive: true, IsSuperuser: in.IsSuperuser}), nil
}

func (m *memRepo) GetByEmail(_ context.Context, email string) (AdminUser, error) {
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return *u, nil
		}
	}
	return AdminUser{}, ErrAdminNotFound
}

func (m *memRepo) GetByID(_ context.Context, id uuid.UUID) (AdminUser, error) {
	u, ok := m.users[id]
	if !ok {
		return AdminUser{}, ErrAdminNotFound
	}
	return *u, nil
}

func (m *memRepo) GetRoles(context.Context, uuid.UUID) ([]Role, error)             { return nil, nil }
func (m *memRepo) GetPermissions(context.Context, uuid.UUID) ([]Permission, error) { return nil, nil }

func (m *memRepo) Update(_ context.Context, id uuid.UUID, in UpdateAdminUserInput) (AdminUser, error) {
	u := m.users[id]
	u.FirstName, u.LastName, u.IsActive = in.FirstName, in.LastName, *in.IsActive
	return *u, nil
}

func (m *memRepo) Delete(_ context.Context, id uuid.UUID) error {
	m.deleted = append(m.deleted, id)
	delete(m.users, id)
	return nil
}

func (m *memRepo) UpdatePassword(_ context.Context, id uuid.UUID, hash string) error {
	m.users[id].PasswordHash = hash
	return nil
}

func (m *memRepo) SetTotpPending(_ context.Context, id uuid.UUID, enc string) error {
	m.users[id].TotpPendingSecret = enc
	return nil
}

func (m *memRepo) EnableTotp(_ context.Context, id uuid.UUID, hashes []byte, step int64) (bool, error) {
	u := m.users[id]
	if u.TotpPendingSecret == "" || u.TotpEnabled {
		return false, nil
	}
	u.TotpSecret, u.TotpPendingSecret, u.TotpEnabled = u.TotpPendingSecret, "", true
	_ = json.Unmarshal(hashes, &u.TotpRecoveryCodes)
	m.lastStep[id] = step
	return true, nil
}

func (m *memRepo) ClaimTotpAttempt(_ context.Context, id uuid.UUID, max int, lock time.Duration) (bool, error) {
	u := m.users[id]
	if u.TotpLockedUntil != nil && m.now().Before(*u.TotpLockedUntil) {
		return false, nil
	}
	if u.TotpLockedUntil != nil {
		m.attempts[id], u.TotpLockedUntil = 0, nil
	}
	m.attempts[id]++
	if m.attempts[id] >= max {
		t := m.now().Add(lock)
		u.TotpLockedUntil = &t
	}
	return true, nil
}

func (m *memRepo) AcceptTotpStep(_ context.Context, id uuid.UUID, step int64) (bool, error) {
	if last, ok := m.lastStep[id]; ok && last >= step {
		return false, nil
	}
	m.lastStep[id] = step
	return true, m.ClearTotpFailures(context.Background(), id)
}

func (m *memRepo) RedeemTotpRecoveryCode(_ context.Context, id uuid.UUID, expected, remaining []byte) (bool, error) {
	if m.redeemErr != nil {
		return false, m.redeemErr
	}
	u := m.users[id]
	current, _ := json.Marshal(u.TotpRecoveryCodes)
	if !bytes.Equal(current, expected) {
		return false, nil
	}
	u.TotpRecoveryCodes = nil
	_ = json.Unmarshal(remaining, &u.TotpRecoveryCodes)
	return true, nil
}

func (m *memRepo) ClearTotpFailures(_ context.Context, id uuid.UUID) error {
	m.attempts[id], m.users[id].TotpLockedUntil = 0, nil
	return nil
}

func (m *memRepo) CreateRole(_ context.Context, in CreateRoleInput) (Role, error) {
	m.roles = append(m.roles, in)
	return Role{Name: in.Name, Slug: in.Slug}, nil
}

type fakeRevoker struct {
	revoked []uuid.UUID
	err     error
}

func (f *fakeRevoker) RevokeAll(_ context.Context, id uuid.UUID) error {
	f.revoked = append(f.revoked, id)
	return f.err
}

type memTokens map[string]uuid.UUID

func (m memTokens) Put(_ context.Context, k string, id uuid.UUID, _ time.Duration) error {
	m[k] = id
	return nil
}

func (m memTokens) Get(_ context.Context, k string) (uuid.UUID, error) {
	id, ok := m[k]
	if !ok {
		return uuid.Nil, auth.ErrTokenNotFound
	}
	return id, nil
}

func (m memTokens) Consume(ctx context.Context, k string) (uuid.UUID, error) {
	id, err := m.Get(ctx, k)
	delete(m, k)
	return id, err
}

func (m memTokens) Delete(_ context.Context, k string) error {
	delete(m, k)
	return nil
}

const testPassword = "correct horse battery staple 42"

type harness struct {
	svc    *Service
	repo   *memRepo
	clock  time.Time
	cipher *secrets.Cipher
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{clock: time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)}
	h.repo = newMemRepo(func() time.Time { return h.clock })
	h.svc = NewService(h.repo)
	h.svc.now = func() time.Time { return h.clock }
	c, err := secrets.NewCipher(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32)))
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	h.cipher = c
	h.svc.SetSecretsCipher(c)
	return h
}

func (h *harness) admin(t *testing.T, email string) *AdminUser {
	t.Helper()
	hash, _ := bcrypt.GenerateFromPassword([]byte(testPassword), bcrypt.MinCost)
	return h.repo.add(AdminUser{Email: email, PasswordHash: string(hash), IsActive: true})
}

// Turns 2FA on directly, with the given plaintext recovery codes
func (h *harness) withTotp(t *testing.T, u *AdminUser, recovery ...string) string {
	t.Helper()
	key, err := totp.Generate(totp.GenerateOpts{Issuer: "test", AccountName: u.Email})
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := h.cipher.Encrypt(key.Secret())
	u.TotpSecret, u.TotpEnabled = enc, true
	for _, rc := range recovery {
		hash, _ := bcrypt.GenerateFromPassword([]byte(rc), bcrypt.MinCost)
		u.TotpRecoveryCodes = append(u.TotpRecoveryCodes, string(hash))
	}
	return key.Secret()
}

func (h *harness) code(t *testing.T, secret string) string {
	t.Helper()
	c, err := hotp.GenerateCodeCustom(secret, uint64(h.clock.Unix()/totpPeriod), hotp.ValidateOpts{Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func wrongCode(right string) string {
	if right == "000000" {
		return "111111"
	}
	return "000000"
}

func TestAuthenticateMatchesEmailInAnyCase(t *testing.T) {
	h := newHarness(t)
	h.admin(t, "jane@shop.test")
	if _, err := h.svc.Authenticate(context.Background(), "  Jane@Shop.TEST ", testPassword, ""); err != nil {
		t.Fatalf("mixed-case login refused: %v", err)
	}
}

func TestCreateAdminStoresTheEmailLowercased(t *testing.T) {
	h := newHarness(t)
	u, err := h.svc.CreateAdmin(context.Background(), " Jane@Shop.test", testPassword, "Jane", "Doe", nil)
	if err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	if u.Email != "jane@shop.test" {
		t.Errorf("stored %q, want it lowercased and trimmed", u.Email)
	}
	if _, err := h.svc.CreateAdmin(context.Background(), "JANE@shop.test", testPassword, "J", "D", nil); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("a case variant of an existing email = %v, want ErrEmailTaken", err)
	}
}

func TestAuthenticateStorageFailureIsNotReportedAsBadCredentials(t *testing.T) {
	boom := errors.New("db down")
	svc := NewService(&failingEmailRepo{err: boom})
	if _, err := svc.Authenticate(context.Background(), "a@b.test", testPassword, ""); !errors.Is(err, boom) {
		t.Errorf("err = %v, want the storage error", err)
	}
}

type failingEmailRepo struct {
	Repository
	err error
}

func (f *failingEmailRepo) GetByEmail(context.Context, string) (AdminUser, error) {
	return AdminUser{}, f.err
}

func TestTotpLoginCodeWorksOnce(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	u := h.admin(t, "a@shop.test")
	secret := h.withTotp(t, u)

	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, ""); !errors.Is(err, ErrTotpRequired) {
		t.Fatalf("no code: err = %v, want ErrTotpRequired", err)
	}
	code := h.code(t, secret)
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, code); err != nil {
		t.Fatalf("valid code refused: %v", err)
	}
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, code); err == nil {
		t.Fatal("the same code logged in twice - replay inside its window")
	}
	h.clock = h.clock.Add(totpPeriod * time.Second)
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, h.code(t, secret)); err != nil {
		t.Fatalf("the next step's code refused: %v", err)
	}
}

func TestTotpLocksOutAfterRepeatedMisses(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	u := h.admin(t, "a@shop.test")
	secret := h.withTotp(t, u)
	bad := wrongCode(h.code(t, secret))

	for i := range totpMaxAttempts {
		if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, bad); err == nil || errors.Is(err, ErrTotpLocked) {
			t.Fatalf("miss %d: err = %v, want a plain failure", i+1, err)
		}
	}
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, h.code(t, secret)); !errors.Is(err, ErrTotpLocked) {
		t.Fatalf("a right code during lockout: err = %v, want ErrTotpLocked", err)
	}
	h.clock = h.clock.Add(totpLockout + time.Second)
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, h.code(t, secret)); err != nil {
		t.Fatalf("after the lockout lapsed: %v", err)
	}
}

func TestTotpSuccessResetsTheMissCount(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	u := h.admin(t, "a@shop.test")
	secret := h.withTotp(t, u)

	for range totpMaxAttempts - 1 {
		_, _ = h.svc.Authenticate(ctx, u.Email, testPassword, wrongCode(h.code(t, secret)))
	}
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, h.code(t, secret)); err != nil {
		t.Fatalf("valid code: %v", err)
	}
	h.clock = h.clock.Add(totpPeriod * time.Second)
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, wrongCode(h.code(t, secret))); errors.Is(err, ErrTotpLocked) {
		t.Fatal("one miss after a success locked the account - the count carried over")
	}
}

func TestRecoveryCodeIsOneUseAndForgivesFormatting(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	u := h.admin(t, "a@shop.test")
	h.withTotp(t, u, "ABCD-EFGH", "JKMN-PQRS")

	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, "abcdefgh"); err != nil {
		t.Fatalf("lowercase, unhyphenated recovery code refused: %v", err)
	}
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, "ABCD-EFGH"); err == nil {
		t.Fatal("a spent recovery code worked again")
	}
	if len(h.repo.users[u.ID].TotpRecoveryCodes) != 1 {
		t.Errorf("%d codes left, want 1", len(h.repo.users[u.ID].TotpRecoveryCodes))
	}
}

func TestRecoveryCodeRedeemedOffAStaleReadFails(t *testing.T) {
	h := newHarness(t)
	u := h.admin(t, "a@shop.test")
	h.withTotp(t, u, "ABCD-EFGH", "JKMN-PQRS")
	stale := *u
	stale.TotpRecoveryCodes = append([]string{}, u.TotpRecoveryCodes...)

	if ok, err := h.svc.verifySecondFactor(context.Background(), u, "JKMN-PQRS"); err != nil || !ok {
		t.Fatalf("first redemption = (%v, %v)", ok, err)
	}
	// A second login that read the list before the first redeemed must not write the spent code back
	if ok, _ := h.svc.verifySecondFactor(context.Background(), &stale, "ABCD-EFGH"); ok {
		t.Fatal("redemption off a stale list succeeded")
	}
	left := h.repo.users[u.ID].TotpRecoveryCodes
	if len(left) != 1 || bcrypt.CompareHashAndPassword([]byte(left[0]), []byte("ABCD-EFGH")) != nil {
		t.Errorf("stored list changed by the stale redemption")
	}
}

func TestNormalizeRecoveryCode(t *testing.T) {
	for in, want := range map[string]string{"abcd-efgh": "ABCD-EFGH", "ABCDEFGH": "ABCD-EFGH", "23456789": "2345-6789"} {
		if got, ok := normalizeRecoveryCode(in); !ok || got != want {
			t.Errorf("normalizeRecoveryCode(%q) = (%q, %v), want %q", in, got, ok, want)
		}
	}
	for _, in := range []string{"123456", "ABCD-EFG", "ABCD-EFGI", "ABCD-EFGH-J"} { // I is outside the alphabet
		if _, ok := normalizeRecoveryCode(in); ok {
			t.Errorf("normalizeRecoveryCode(%q) accepted a non-recovery code", in)
		}
	}
}

func TestTotpSetupRefusedWhileEnabled(t *testing.T) {
	h := newHarness(t)
	u := h.admin(t, "a@shop.test")
	h.withTotp(t, u)
	if _, _, err := h.svc.TotpSetup(context.Background(), u.ID); !errors.Is(err, ErrTotpAlreadyEnabled) {
		t.Errorf("err = %v, want ErrTotpAlreadyEnabled", err)
	}
	if _, err := h.svc.TotpEnable(context.Background(), u.ID, "123456"); !errors.Is(err, ErrTotpAlreadyEnabled) {
		t.Errorf("TotpEnable err = %v, want ErrTotpAlreadyEnabled", err)
	}
}

func TestTotpEnableSpendsTheEnablingCode(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	u := h.admin(t, "a@shop.test")
	secret, _, err := h.svc.TotpSetup(ctx, u.ID)
	if err != nil {
		t.Fatalf("TotpSetup: %v", err)
	}
	code := h.code(t, secret)
	codes, err := h.svc.TotpEnable(ctx, u.ID, code)
	if err != nil || len(codes) != 10 {
		t.Fatalf("TotpEnable = (%d codes, %v)", len(codes), err)
	}
	if _, err := h.svc.Authenticate(ctx, u.Email, testPassword, code); err == nil {
		t.Fatal("the code that enabled 2FA also logged in")
	}
}

func TestDeactivatingAnAdminEndsTheirSessions(t *testing.T) {
	h := newHarness(t)
	rev := &fakeRevoker{}
	h.svc.refresh = rev
	u := h.admin(t, "a@shop.test")
	off, on := false, true

	if _, err := h.svc.Update(context.Background(), u.ID, UpdateAdminUserInput{FirstName: "New"}); err != nil {
		t.Fatal(err)
	}
	if _, err := h.svc.Update(context.Background(), u.ID, UpdateAdminUserInput{IsActive: &on}); err != nil {
		t.Fatal(err)
	}
	if len(rev.revoked) != 0 {
		t.Fatalf("revoked %v on edits that left the account active", rev.revoked)
	}
	if _, err := h.svc.Update(context.Background(), u.ID, UpdateAdminUserInput{IsActive: &off}); err != nil {
		t.Fatal(err)
	}
	if len(rev.revoked) != 1 || rev.revoked[0] != u.ID {
		t.Errorf("revoked %v, want [%s]", rev.revoked, u.ID)
	}
}

func TestDeleteRevokesFirstAndStopsWhenThatFails(t *testing.T) {
	h := newHarness(t)
	rev := &fakeRevoker{err: errors.New("valkey down")}
	h.svc.refresh = rev
	u := h.admin(t, "a@shop.test")

	if err := h.svc.Delete(context.Background(), u.ID); err == nil {
		t.Fatal("delete went through with the sessions still live")
	}
	if len(h.repo.deleted) != 0 {
		t.Fatal("the row was deleted although its sessions could not be revoked")
	}
	rev.err = nil
	if err := h.svc.Delete(context.Background(), u.ID); err != nil {
		t.Fatal(err)
	}
	if len(h.repo.deleted) != 1 {
		t.Errorf("deleted %v, want the admin", h.repo.deleted)
	}
}

func TestResetTokenIsSpentEvenWhenRevokeFails(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	u := h.admin(t, "a@shop.test")
	store := auth.NewPasswordResetStoreFor(memTokens{}, "admin")
	h.svc.SetPasswordReset(store, nil, nil, "")
	rev := &fakeRevoker{err: errors.New("valkey down")}
	h.svc.refresh = rev

	token, err := store.Create(ctx, u.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := h.svc.ResetPassword(ctx, token, "another long passphrase 99"); err == nil {
		t.Fatal("a failed session revoke must be reported")
	}
	rev.err = nil
	if err := h.svc.ResetPassword(ctx, token, "a third long passphrase 77"); err == nil {
		t.Fatal("the reset token worked a second time")
	}
}

func TestCreateRoleNeedsAGrant(t *testing.T) {
	h := newHarness(t)
	if _, err := h.svc.CreateRole(context.Background(), CreateRoleInput{Name: "Empty", Slug: "empty"}); err == nil {
		t.Fatal("created a role granting nothing")
	}
	if len(h.repo.roles) != 0 {
		t.Error("the empty role reached the repository")
	}
}
