package money

import (
	"math"
	"testing"
)

func TestFormat(t *testing.T) {
	for _, tc := range []struct {
		minor    int64
		decimals int
		want     string
	}{
		{123456, 2, "1234.56"}, {5, 2, "0.05"}, {-5, 2, "-0.05"}, {0, 2, "0.00"}, {5000, 0, "5000"},
		{1234, 3, "1.234"}, {-150, 2, "-1.50"}, {math.MinInt64, 2, "-92233720368547758.08"},
		{90071992547409930, 2, "900719925474099.30"}, // past float64's exact range, where %.2f of cents/100 drifts
	} {
		if got := Format(tc.minor, tc.decimals); got != tc.want {
			t.Errorf("Format(%d, %d) = %q, want %q", tc.minor, tc.decimals, got, tc.want)
		}
	}
}

func TestISODecimals(t *testing.T) {
	for code, want := range map[string]int{"JPY": 0, "jpy": 0, "KWD": 3, "USD": 2, "RSD": 2, "": 2} {
		if got := ISODecimals(code); got != want {
			t.Errorf("ISODecimals(%q) = %d, want %d", code, got, want)
		}
	}
}

func TestFormatCodeUsesTheCurrencysExponent(t *testing.T) {
	for _, c := range []struct {
		minor int64
		code  string
		want  string
	}{
		{120000, "JPY", "JPY 120000"},
		{1234, "usd", "USD 12.34"},
		{12345, "KWD", "KWD 12.345"},
		{-5, "EUR", "EUR -0.05"},
		{1234, "", "12.34"},
	} {
		if got := FormatCode(c.minor, c.code); got != c.want {
			t.Errorf("FormatCode(%d, %q) = %q, want %q", c.minor, c.code, got, c.want)
		}
	}
}
