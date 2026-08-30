package sdk

// Admin-UI language packs. A plugin embeds a translation catalog and registers it here;
// the host serves it to the dashboard at runtime (GET /api/admin-ui/i18n/*), so adding a
// language never rebuilds the dashboard image - installing the pack is the whole job
//
// Messages is a Lingui minimal-format catalog: a flat JSON object of message id to ICU
// MessageFormat string. Pack authors translate the template the dashboard publishes
// (apps/dashboard/src/locales/en.po) - ids missing from a pack fall back to the English
// that is inlined in the dashboard bundle, so a pack built against an older dashboard
// degrades to partial coverage instead of breaking

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// One locale per registration - a pack shipping three languages registers three times
type AdminTranslationsRegistration struct {
	// Locale is a BCP 47 tag ("sr", "pt-BR") - it names the catalog URL and must be
	// unique across every installed pack
	Locale string
	// DisplayName is shown in the dashboard's language picker, written in its own
	// language ("Srpski", not "Serbian")
	DisplayName string
	// Direction is "ltr" or "rtl"; empty means ltr
	Direction string
	// Messages is the catalog: {"<id>": "<ICU message>", ...}
	Messages json.RawMessage
}

var localePattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Za-z0-9]{2,8})*$`)

// Call from init(). Panics on a malformed registration - like a bad manifest, a broken
// language pack deserves to fail at boot, not to 404 when an admin picks the language
func (p *Plugin) RegisterAdminTranslations(reg AdminTranslationsRegistration) {
	if err := validateAdminTranslations(reg); err != nil {
		panic(fmt.Sprintf("sdk.RegisterAdminTranslations(%q): %v", p.slug, err))
	}
	withHost(func(h Host) error { return h.RegisterAdminTranslations(p.slug, reg) })
}

func validateAdminTranslations(reg AdminTranslationsRegistration) error {
	if !localePattern.MatchString(reg.Locale) {
		return fmt.Errorf("locale %q is not a BCP 47-shaped tag (lowercase language, optional subtags)", reg.Locale)
	}
	if strings.TrimSpace(reg.DisplayName) == "" {
		return fmt.Errorf("empty DisplayName")
	}
	switch reg.Direction {
	case "", "ltr", "rtl":
	default:
		return fmt.Errorf("direction %q must be \"ltr\" or \"rtl\"", reg.Direction)
	}
	var msgs map[string]string
	if err := json.Unmarshal(reg.Messages, &msgs); err != nil {
		return fmt.Errorf("messages must be a JSON object of id to ICU string: %w", err)
	}
	if len(msgs) == 0 {
		return fmt.Errorf("empty catalog")
	}
	return nil
}
