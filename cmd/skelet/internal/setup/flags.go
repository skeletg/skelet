package setup

import (
	"fmt"
	"os"
	"slices"
	"strings"
)

// A list flag: set is whether it was given at all, since --apps none is a choice and no --apps is not
type list struct {
	set   bool
	items []string
}

type flags struct {
	apps, dockerApps, tools list
	keepCode, yes, noBoot   bool
	// profile is the file answers are replayed from; they are always saved to .skelet-profile
	profile string
	// shop and template ask for a storefront of the project's own, copied from a template - either one is enough
	shop, template string
}

// usage is a message for the person at the terminal, printed as it stands - worded the way the bash setup's were
type usage string

func (u usage) Error() string { return string(u) }

func parseFlags(args []string, c catalog) (flags, error) {
	var f flags
	all := join(c.apps())
	for i := 0; i < len(args); i++ {
		name, val, inline := strings.Cut(args[i], "=")
		value := func() (string, bool) {
			if inline {
				return val, val != ""
			}
			if i+1 < len(args) && args[i+1] != "" {
				i++
				return args[i], true
			}
			return "", false
		}
		switch name {
		case "--all":
			f.apps = list{true, c.apps()}
		case "--backend-only":
			f.apps = list{true, nil}
		case "--apps", "--docker-apps":
			v, ok := value()
			if !ok {
				return f, usagef("%s needs a value (e.g. %s or none)", name, all)
			}
			l, err := pick(v, c.apps(), "app")
			if err != nil {
				return f, err
			}
			if name == "--apps" {
				f.apps = l
			} else {
				f.dockerApps = l
			}
		case "--tools":
			v, ok := value()
			if !ok {
				return f, usagef("--tools needs a value (e.g. %s or none)", join(tools))
			}
			l, err := pick(v, tools, "tool")
			if err != nil {
				return f, err
			}
			f.tools = l
		case "--shop":
			v, ok := value()
			if !ok {
				return f, usage("--shop needs a name for your storefront (e.g. shop)")
			}
			if why := shopUnusable(c, v); why != "" {
				return f, usagef("Can't name the shop %q - %s", v, why)
			}
			f.shop = v
		case "--template":
			v, ok := value()
			if !ok || !slices.Contains(c.templateNames(), v) {
				return f, usagef("--template takes a template (valid: %s)", strings.Join(c.templateNames(), ", "))
			}
			f.template = v
		case "--keep-code":
			f.keepCode = true
		case "--yes", "-y":
			f.yes = true
		case "--no-boot":
			f.noBoot = true
		case "--profile":
			v, ok := value()
			if !ok {
				return f, usage("--profile needs a file")
			}
			if fi, err := os.Stat(v); err != nil || !fi.Mode().IsRegular() {
				return f, usagef("No such profile: %s", v)
			}
			f.profile, f.yes = v, true
		default:
			return f, usagef("Unknown flag: %s (try --all, --backend-only, --apps LIST, --docker-apps LIST, --tools LIST, --shop NAME, --template NAME, --keep-code, --yes, --profile FILE)", args[i])
		}
	}
	return f, nil
}

func usagef(format string, a ...any) error { return usage(fmt.Sprintf(format, a...)) }

func pick(v string, valid []string, what string) (list, error) {
	if v == "none" {
		return list{true, nil}, nil
	}
	items := split(v)
	for _, it := range items {
		if !slices.Contains(valid, it) {
			return list{}, usagef("Unknown %s %q (valid: %s, none)", what, it, strings.Join(valid, ", "))
		}
	}
	return list{true, items}, nil
}
