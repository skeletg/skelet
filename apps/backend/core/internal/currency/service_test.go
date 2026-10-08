package currency

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type guardRepo struct {
	Repository
	current     Currency
	hasAmounts  bool
	channels    int64
	amountsAsks int
	updated     bool
}

func (g *guardRepo) GetByCode(context.Context, string) (Currency, error) { return g.current, nil }
func (g *guardRepo) HasAmounts(context.Context, string) (bool, error) {
	g.amountsAsks++
	return g.hasAmounts, nil
}
func (g *guardRepo) CountChannels(context.Context, string) (int64, error) { return g.channels, nil }
func (g *guardRepo) Update(context.Context, UpdateCurrencyInput) (Currency, error) {
	g.updated = true
	return g.current, nil
}

func TestUpdateGuards(t *testing.T) {
	off := false
	tests := []struct {
		name  string
		repo  guardRepo
		input UpdateCurrencyInput
		want  error
	}{
		{"decimals change with amounts", guardRepo{current: Currency{Decimals: 2, IsActive: true}, hasAmounts: true}, UpdateCurrencyInput{Decimals: decimals(3)}, ErrDecimalsLocked},
		{"decimals change without amounts", guardRepo{current: Currency{Decimals: 2, IsActive: true}}, UpdateCurrencyInput{Decimals: decimals(3)}, nil},
		{"unchanged decimals with amounts", guardRepo{current: Currency{Decimals: 2, IsActive: true}, hasAmounts: true}, UpdateCurrencyInput{Decimals: decimals(2)}, nil},
		{"deactivate a channel's currency", guardRepo{current: Currency{IsActive: true}, channels: 1}, UpdateCurrencyInput{IsActive: &off}, ErrCurrencyUsedByChannel},
		{"deactivate an unused currency", guardRepo{current: Currency{IsActive: true}}, UpdateCurrencyInput{IsActive: &off}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.repo
			tt.input.Code = "XTS"
			_, err := NewService(&repo).Update(context.Background(), tt.input)
			if !errors.Is(err, tt.want) || (tt.want == nil && err != nil) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
			if repo.updated != (tt.want == nil) {
				t.Errorf("updated = %v", repo.updated)
			}
		})
	}
}

func TestUnchangedDecimalsSkipTheReferenceScan(t *testing.T) {
	repo := &guardRepo{current: Currency{Decimals: 2}, hasAmounts: true}
	if _, err := NewService(repo).Update(context.Background(), UpdateCurrencyInput{Code: "XTS", Decimals: decimals(2)}); err != nil {
		t.Fatal(err)
	}
	if repo.amountsAsks != 0 {
		t.Errorf("scanned for amounts %d time(s) on an unchanged value", repo.amountsAsks)
	}
}

func TestValidationSentinels(t *testing.T) {
	svc := NewService(&guardRepo{})
	ctx := context.Background()
	big := map[string]string{"k": strings.Repeat("x", maxMetadataValueLength+1)}
	many := map[string]string{}
	for i := range maxMetadataKeys + 1 {
		many[strings.Repeat("k", i+1)] = "v"
	}
	cases := []struct {
		name  string
		input CreateCurrencyInput
		want  error
	}{
		{"blank name", CreateCurrencyInput{Code: "XTS", Name: " "}, ErrNameRequired},
		{"bad symbol position", CreateCurrencyInput{Code: "XTS", Name: "x", SymbolPosition: "middle"}, ErrInvalidSymbolPosition},
		{"decimals above range", CreateCurrencyInput{Code: "XTS", Name: "x", Decimals: MaxDecimals + 1}, ErrInvalidDecimals},
		{"decimals below range", CreateCurrencyInput{Code: "XTS", Name: "x", Decimals: MinDecimals - 1}, ErrInvalidDecimals},
		{"oversized metadata value", CreateCurrencyInput{Code: "XTS", Name: "x", Metadata: big}, ErrMetadataTooLarge},
		{"too many metadata keys", CreateCurrencyInput{Code: "XTS", Name: "x", Metadata: many}, ErrMetadataTooLarge},
	}
	for _, c := range cases {
		if _, err := svc.Create(ctx, c.input); !errors.Is(err, c.want) {
			t.Errorf("%s: %v, want %v", c.name, err, c.want)
		}
	}
	if _, err := svc.Update(ctx, UpdateCurrencyInput{Code: "XTS", Metadata: &big}); !errors.Is(err, ErrMetadataTooLarge) {
		t.Errorf("update metadata: %v", err)
	}
	if err := svc.SetRate(ctx, "XTS", 0); !errors.Is(err, ErrInvalidRate) {
		t.Errorf("zero rate: %v", err)
	}
}
