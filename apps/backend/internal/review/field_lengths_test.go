package review

import (
	"errors"
	"strings"
	"testing"
)

func TestValidateFieldLengths(t *testing.T) {
	cases := []struct {
		name    string
		in      SubmitReviewInput
		wantErr bool
	}{
		{name: "empty ok", in: SubmitReviewInput{}},
		{
			name: "normal ok",
			in:   SubmitReviewInput{Title: "Great lens", Body: "Sharp and fast.", GuestName: "Ada", GuestEmail: "ada@example.com"},
		},
		{name: "title at cap ok", in: SubmitReviewInput{Title: strings.Repeat("t", MaxReviewTitleLen)}},
		{name: "title over cap", in: SubmitReviewInput{Title: strings.Repeat("t", MaxReviewTitleLen+1)}, wantErr: true},
		{name: "body over cap", in: SubmitReviewInput{Body: strings.Repeat("b", MaxReviewBodyLen+1)}, wantErr: true},
		{name: "guest name over cap", in: SubmitReviewInput{GuestName: strings.Repeat("n", MaxGuestNameLen+1)}, wantErr: true},
		{name: "guest email over cap", in: SubmitReviewInput{GuestEmail: strings.Repeat("e", MaxGuestEmailLen+1)}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateFieldLengths(tc.in)
			if tc.wantErr {
				if !errors.Is(err, ErrFieldTooLong) {
					t.Fatalf("want ErrFieldTooLong, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("want nil, got %v", err)
			}
		})
	}
}
