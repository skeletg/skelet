// Package money is integer minor-unit money - rounding arithmetic for price times quantity, and formatting for people to read, never through a float that loses cents
package money

import (
	"strconv"
	"strings"
)

// ISO 4217 currencies whose minor unit isn't a hundredth - everything else has two decimals
var isoDecimals = map[string]int{
	"BIF": 0, "CLP": 0, "DJF": 0, "GNF": 0, "ISK": 0, "JPY": 0, "KMF": 0, "KRW": 0, "PYG": 0,
	"RWF": 0, "UGX": 0, "UYI": 0, "VND": 0, "VUV": 0, "XAF": 0, "XOF": 0, "XPF": 0,
	"BHD": 3, "IQD": 3, "JOD": 3, "KWD": 3, "LYD": 3, "OMR": 3, "TND": 3,
	"CLF": 4, "UYW": 4,
}

// The minor-unit exponent ISO 4217 gives code - for callers holding only the code, since a configured currency's own count wins where it's known
func ISODecimals(code string) int {
	if d, ok := isoDecimals[strings.ToUpper(code)]; ok {
		return d
	}
	return 2
}

// Format renders minor units with decimals digits after the point - 123456 at 2 is "1234.56", at 0 "123456"
func Format(minor int64, decimals int) string {
	sign := ""
	u := uint64(minor)
	if minor < 0 {
		sign, u = "-", uint64(-(minor+1))+1
	}
	digits := strconv.FormatUint(u, 10)
	if decimals <= 0 {
		return sign + digits
	}
	if len(digits) <= decimals {
		digits = strings.Repeat("0", decimals-len(digits)+1) + digits
	}
	cut := len(digits) - decimals
	return sign + digits[:cut] + "." + digits[cut:]
}

// "JPY 1200", "USD 12.34" - the code's ISO exponent, for text read by people or a model
func FormatCode(minor int64, code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	if code == "" {
		return Format(minor, 2)
	}
	return code + " " + Format(minor, ISODecimals(code))
}
