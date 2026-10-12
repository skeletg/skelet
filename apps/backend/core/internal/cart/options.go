package cart

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/skeletg/skelet/apps/backend/core/lib/money"
)

const maxOptionTextRunes = 2000

type ProductOptionsResolution struct { // built by the catalog bridge, so the cart never trusts the client for option structure or pricing
	ProductID   uuid.UUID
	ProductName string
	UnitPrice   int64 // base line price (cents) before options
	BasePrice   int64 // resolved channel unit price (cents) - the base for percent adjustments
	Options     []ProductOptionConfig
}

type ProductOptionConfig struct { // snapshot of the catalog row
	ID             uuid.UUID
	InputType      string // field|area|dropdown|radio|checkbox|multiselect|date|date_time|time|file
	IsRequired     bool
	Title          string
	SortOrder      int32 // authored order - the resolution walks this order, and a variant's merged parent options interleave by it
	MaxChars       *int32
	MaxFileBytes   *int64 // FILE-input constraint, nil = uncapped
	FileExtensions string // FILE-input constraint, comma-separated allow-list, empty = no constraint
	PriceValue     *int32 // option-level adjustment for text, date and file options, since select-family carries adjustments on values instead - nil for none
	PriceType      string // "fixed" or "percent"
	// The FIXED surcharge has no amount for this channel's currency, so filling the option is refused
	PriceUnavailable bool
	Values           []ProductOptionValueConfig
	VisibleIf        *OptionVisibilityGroup // gates the option on EARLIER picks (nil = always visible), see options_visibility.go
}

type FileAsset struct { // enough to validate an upload against the option's constraints and snapshot a durable reference on the line
	Filename  string
	MimeType  string
	SizeBytes int64
	URL       string
}

type ProductOptionValueConfig struct { // one select-family value, snapshotted
	ID                 uuid.UUID
	Title              string
	PriceValue         int32
	PriceType          string // "fixed" | "percent"
	Quantity           float64
	HasSource          bool // backed by a SIMPLE product, so the contribution is that product's price plus PriceValue, forced fixed
	SourceProductID    uuid.UUID
	SourceProductPrice int64
	SourceUnavailable  bool     // the linked source isn't sold on this channel, so the value can't be picked
	PriceUnavailable   bool     // no surcharge for this channel's currency, so the value can't be picked
	ForcedLineQty      *float64 // picking this value forces the line quantity and locks it against manual edits - nil means no constraint
}

// Trusts nothing from the client beyond chosen IDs and text - two picks forcing different quantities is ErrProductOptionQtyConflict, and fileAssets is pre-resolved by the service so this stays I/O-free
func buildConfiguredLineOptions(res ProductOptionsResolution, inputs []CartLineOptionInput, fileAssets map[uuid.UUID]FileAsset) ([]CartLineOption, int64, *float64, error) {
	cfgByID := make(map[uuid.UUID]ProductOptionConfig, len(res.Options))
	for _, o := range res.Options {
		cfgByID[o.ID] = o
	}
	visible := visibleOptionIDs(res, inputs)
	supplied := make(map[uuid.UUID]CartLineOptionInput, len(inputs))
	for _, in := range inputs {
		if _, ok := cfgByID[in.OptionID]; !ok {
			return nil, 0, nil, fmt.Errorf("%w: %s", ErrProductOptionUnknown, in.OptionID)
		}
		if !visible[in.OptionID] && pickCarriesValue(in) {
			return nil, 0, nil, fmt.Errorf("%w: option %s", ErrProductOptionHidden, in.OptionID)
		}
		if _, dup := supplied[in.OptionID]; dup {
			return nil, 0, nil, fmt.Errorf("%w: option %s supplied twice", ErrProductOptionInvalid, in.OptionID)
		}
		supplied[in.OptionID] = in
	}

	var (
		out       []CartLineOption
		delta     int64
		sort      int32
		forcedQty *float64
	)
	for _, opt := range res.Options { // the product's own option set rather than the input, so omitted required options are caught and output order is stable
		if !visible[opt.ID] {
			continue // hidden: no required-check, no price delta, no snapshot
		}
		in, ok := supplied[opt.ID]
		if isSelectInput(opt.InputType) {
			if !ok || len(in.ValueIDs) == 0 {
				if opt.IsRequired {
					return nil, 0, nil, fmt.Errorf("%w: option %s", ErrProductOptionMissing, opt.ID)
				}
				continue
			}
			if isSingleSelectInput(opt.InputType) && len(in.ValueIDs) > 1 {
				return nil, 0, nil, fmt.Errorf("%w: option %s allows one value, got %d", ErrProductOptionCardinality, opt.ID, len(in.ValueIDs))
			}
			valByID := make(map[uuid.UUID]ProductOptionValueConfig, len(opt.Values))
			for _, v := range opt.Values {
				valByID[v.ID] = v
			}
			seen := make(map[uuid.UUID]bool, len(in.ValueIDs))
			for _, vid := range in.ValueIDs {
				v, found := valByID[vid]
				if !found {
					return nil, 0, nil, fmt.Errorf("%w: value %s not on option %s", ErrProductOptionInvalid, vid, opt.ID)
				}
				if seen[vid] {
					continue // ignore duplicate value IDs in one option
				}
				if v.SourceUnavailable {
					return nil, 0, nil, fmt.Errorf("%w: value %s", ErrOptionValueUnavailable, vid)
				}
				if v.PriceUnavailable {
					return nil, 0, nil, fmt.Errorf("%w: value %s", ErrOptionPriceUnavailable, vid)
				}
				seen[vid] = true
				if v.ForcedLineQty != nil {
					if forcedQty != nil && *forcedQty != *v.ForcedLineQty {
						return nil, 0, nil, fmt.Errorf("%w: %v vs %v", ErrProductOptionQtyConflict, *forcedQty, *v.ForcedLineQty)
					}
					q := *v.ForcedLineQty
					forcedQty = &q
				}
				rowDelta := money.LineTotal(valueContribution(res.BasePrice, v), v.Quantity)
				delta += rowDelta
				valID := vid
				lo := CartLineOption{
					ProductOptionID: opt.ID,
					OptionValueID:   &valID,
					InputType:       opt.InputType,
					OptionTitle:     opt.Title,
					ValueLabel:      v.Title,
					PriceDelta:      rowDelta,
					Quantity:        v.Quantity,
					SortOrder:       sort,
				}
				if v.HasSource && v.SourceProductID != uuid.Nil {
					src := v.SourceProductID
					lo.SourceProductID = &src
				}
				out = append(out, lo)
				sort++
			}
			continue
		}

		if opt.InputType == "file" { // the client supplies an asset id, and the asset's real server-side filename/size/extension are what get validated
			if !ok || in.FileAssetID == nil {
				if opt.IsRequired {
					return nil, 0, nil, fmt.Errorf("%w: option %s", ErrProductOptionMissing, opt.ID)
				}
				continue
			}
			asset, found := fileAssets[*in.FileAssetID]
			if !found {
				return nil, 0, nil, fmt.Errorf("%w: option %s file upload not found", ErrProductOptionInvalid, opt.ID)
			}
			if err := validateFilePick(opt, asset); err != nil {
				return nil, 0, nil, err
			}
			if opt.PriceUnavailable {
				return nil, 0, nil, fmt.Errorf("%w: option %s", ErrOptionPriceUnavailable, opt.ID)
			}
			c := optionContribution(res.BasePrice, opt)
			delta += c
			out = append(out, CartLineOption{
				ProductOptionID: opt.ID,
				InputType:       opt.InputType,
				OptionTitle:     opt.Title,
				TextValue:       asset.Filename, // display label
				FileURL:         asset.URL,
				PriceDelta:      c,
				Quantity:        1,
				SortOrder:       sort,
			})
			sort++
			continue
		}

		text := strings.TrimSpace(in.TextValue) // text/date/time - opaque value, priced by the option-level adjustment
		if !ok || text == "" {
			if opt.IsRequired {
				return nil, 0, nil, fmt.Errorf("%w: option %s", ErrProductOptionMissing, opt.ID)
			}
			continue
		}
		limit := int32(maxOptionTextRunes) // an uncapped option still gets a ceiling, or the request body limit is the only thing bounding what a line snapshots
		if isTextInput(opt.InputType) && opt.MaxChars != nil {
			limit = *opt.MaxChars
		}
		if int32(utf8.RuneCountInString(text)) > limit {
			return nil, 0, nil, fmt.Errorf("%w: option %s exceeds %d characters", ErrProductOptionInvalid, opt.ID, limit)
		}
		if err := validateDateValue(opt.InputType, text); err != nil {
			return nil, 0, nil, fmt.Errorf("%w: option %s: %w", ErrProductOptionInvalid, opt.ID, err)
		}
		if opt.PriceUnavailable {
			return nil, 0, nil, fmt.Errorf("%w: option %s", ErrOptionPriceUnavailable, opt.ID)
		}
		c := optionContribution(res.BasePrice, opt)
		delta += c
		out = append(out, CartLineOption{
			ProductOptionID: opt.ID,
			InputType:       opt.InputType,
			OptionTitle:     opt.Title,
			TextValue:       text,
			PriceDelta:      c,
			Quantity:        1,
			SortOrder:       sort,
		})
		sort++
	}
	return out, delta, forcedQty, nil
}

// HasVisiblyRequiredOptions reports whether a plain no-picks add must be rejected - a required option hidden behind an unsatisfied visible_if doesn't count, since the hidden state is exactly how a customer would legitimately buy without it
func HasVisiblyRequiredOptions(res ProductOptionsResolution) bool {
	visible := visibleOptionIDs(res, nil)
	for _, opt := range res.Options {
		if opt.IsRequired && visible[opt.ID] {
			return true
		}
	}
	return false
}

func validateFilePick(opt ProductOptionConfig, asset FileAsset) error {
	if opt.MaxFileBytes != nil && *opt.MaxFileBytes > 0 && asset.SizeBytes > *opt.MaxFileBytes {
		return fmt.Errorf("%w: option %s file exceeds %d bytes", ErrProductOptionInvalid, opt.ID, *opt.MaxFileBytes)
	}
	allow := parseExtensionAllowList(opt.FileExtensions)
	if len(allow) == 0 {
		return nil // no extension constraint
	}
	ext := fileExtension(asset.Filename)
	if ext == "" || !allow[ext] {
		return fmt.Errorf("%w: option %s file type %q not allowed", ErrProductOptionInvalid, opt.ID, ext)
	}
	return nil
}

func parseExtensionAllowList(raw string) map[string]bool { // turns "jpg, PNG, .pdf" into lowercase, dot-stripped extensions
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	out := make(map[string]bool)
	for part := range strings.SplitSeq(raw, ",") {
		e := strings.ToLower(strings.TrimSpace(part))
		e = strings.TrimPrefix(e, ".")
		if e != "" {
			out[e] = true
		}
	}
	return out
}

func fileExtension(filename string) string {
	_, ext, found := strings.CutLast(filename, ".")
	if !found || ext == "" {
		return ""
	}
	return strings.ToLower(ext)
}

func valueContribution(basePrice int64, v ProductOptionValueConfig) int64 { // source-product price plus delta when backed by a product, else its own fixed or percent adjustment
	if v.HasSource {
		return v.SourceProductPrice + int64(v.PriceValue)
	}
	if v.PriceType == "percent" {
		return percentOf(basePrice, v.PriceValue)
	}
	return int64(v.PriceValue)
}

func optionContribution(basePrice int64, o ProductOptionConfig) int64 {
	if o.PriceValue == nil {
		return 0
	}
	if o.PriceType == "percent" {
		return percentOf(basePrice, *o.PriceValue)
	}
	return int64(*o.PriceValue)
}

func percentOf(base int64, pct int32) int64 { // rounded half away from zero
	n := base * int64(pct)
	if n >= 0 {
		return (n + 50) / 100
	}
	return -((-n + 50) / 100)
}

func isSelectInput(t string) bool {
	switch t {
	case "dropdown", "radio", "checkbox", "multiselect":
		return true
	}
	return false
}

func isSingleSelectInput(t string) bool { return t == "dropdown" || t == "radio" }

func isTextInput(t string) bool { return t == "field" || t == "area" }

func validateDateValue(inputType, text string) error { // no-op for non-date types - garbage in a date field would otherwise snapshot straight onto the order
	var layouts []string
	switch inputType {
	case "date":
		layouts = []string{"2006-01-02"}
	case "time":
		layouts = []string{"15:04", "15:04:05"}
	case "date_time":
		layouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", time.RFC3339}
	default:
		return nil
	}
	for _, l := range layouts {
		if _, err := time.Parse(l, text); err == nil {
			return nil
		}
	}
	return fmt.Errorf("invalid %s value %q", inputType, text)
}
