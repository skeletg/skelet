package country

import (
	"context"
	"slices"
	"testing"

	"github.com/skeletg/skelet/apps/backend/core/lib/config"
)

func phoneCfg(mode, allowed, def string) fakeConfig {
	return fakeConfig{strings: map[string]string{
		CfgPhoneCountries:   mode,
		CfgAllowedCountries: allowed,
		CfgDefaultCountry:   def,
	}}
}

func TestPhoneRulesFollowTheSetting(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name        string
		cfg         fakeConfig
		wantDefault string
		wantAllowed []string
	}{
		{"any country takes every number", phoneCfg(PhoneCountriesAny, "US,CA", "US"), "US", nil},
		{"unset means any", phoneCfg("", "US,CA", "US"), "US", nil},
		{"allowed keeps phones to the allow-list", phoneCfg(PhoneCountriesAllowed, "us, ca", "US"), "US", []string{"US", "CA"}},
		// One country: every phone is that country's, even a national one typed with no address
		{"a single allowed country becomes the default", phoneCfg(PhoneCountriesAllowed, "RS", "US"), "RS", []string{"RS"}},
		// No allow-list means the store sells to every active country - the fake repository has none
		{"allowed with no allow-list and no active countries takes every number", phoneCfg(PhoneCountriesAllowed, "", "DE"), "DE", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newSvc(tc.cfg).PhoneRules(ctx, config.DefaultScope)
			if r.Default != tc.wantDefault || !slices.Equal(r.Allowed, tc.wantAllowed) {
				t.Errorf("rules = %+v, want default %q allowed %v", r, tc.wantDefault, tc.wantAllowed)
			}
		})
	}
	if r := newSvc(nil).PhoneRules(ctx, config.DefaultScope); r.Default != "" || r.Allowed != nil {
		t.Errorf("without config = %+v, want empty rules", r)
	}
}

func TestPhoneSettingsLockASingleCountryStore(t *testing.T) {
	s, err := newSvc(phoneCfg(PhoneCountriesAllowed, "RS", "RS")).PhoneSettings(context.Background(), config.DefaultScope)
	if err != nil {
		t.Fatal(err)
	}
	if s.Locked != "RS" || len(s.Formats) != 1 || s.Formats[0].CallingCode != "+381" || s.Formats[0].NationalExample != "060 1234567" {
		t.Fatalf("settings = %+v, want RS locked with its format", s)
	}
}

func TestPhoneSettingsListTheAllowedCountriesInOrder(t *testing.T) {
	s, err := newSvc(phoneCfg(PhoneCountriesAny, "DE,US", "DE")).PhoneSettings(context.Background(), config.DefaultScope)
	if err != nil {
		t.Fatal(err)
	}
	var codes []string
	for _, f := range s.Formats {
		codes = append(codes, f.Country)
	}
	if s.Locked != "" || !slices.Equal(codes, []string{"DE", "US"}) {
		t.Fatalf("locked %q, formats %v, want none locked and DE, US", s.Locked, codes)
	}
}

// With no allow-list the store sells to every active country, so those are the ones a form offers
type activeRepo struct{ fakeRepo }

func (activeRepo) List(context.Context, bool) ([]Country, error) {
	return []Country{{Code: "US", IsActive: true}, {Code: "GB", IsActive: true}}, nil
}

func TestPhoneSettingsFallBackToTheActiveCountries(t *testing.T) {
	svc := NewService(activeRepo{})
	svc.SetConfigService(phoneCfg(PhoneCountriesAny, "", "US"))
	s, err := svc.PhoneSettings(context.Background(), config.DefaultScope)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Formats) != 2 || s.Formats[0].Country != "US" || s.Formats[1].Country != "GB" {
		t.Fatalf("formats = %+v, want US and GB", s.Formats)
	}
}

// With no allow-list, "allowed" keeps phones to the active countries - the store sells to those
func TestPhoneRulesWithNoAllowListKeepToTheActiveCountries(t *testing.T) {
	svc := NewService(activeRepo{})
	svc.SetConfigService(phoneCfg(PhoneCountriesAllowed, "", "US"))
	r := svc.PhoneRules(context.Background(), config.DefaultScope)
	if !slices.Equal(r.Allowed, []string{"US", "GB"}) || r.Default != "US" {
		t.Fatalf("rules = %+v, want the active US and GB", r)
	}
	// "any" stays any, active countries or not
	svc.SetConfigService(phoneCfg(PhoneCountriesAny, "", "US"))
	if r := svc.PhoneRules(context.Background(), config.DefaultScope); r.Allowed != nil {
		t.Fatalf("any = %+v, want no restriction", r)
	}
}

func TestCallingCodeIsDerived(t *testing.T) {
	cases := []struct{ code, typed, want string }{
		{"RS", "+38", "+381"},  // a typo on a known country heals
		{"RS", "", "+381"},     // and a blank one fills
		{"XX", "+999", "+999"}, // an unknown code keeps what was typed
	}
	for _, tc := range cases {
		if got := callingCodeFor(tc.code, tc.typed); got != tc.want {
			t.Errorf("callingCodeFor(%q, %q) = %q, want %q", tc.code, tc.typed, got, tc.want)
		}
	}
}
