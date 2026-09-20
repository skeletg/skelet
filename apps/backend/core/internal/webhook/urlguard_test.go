package webhook

import "testing"

func TestValidateTargetURL(t *testing.T) {
	cases := []struct {
		name         string
		url          string
		allowPrivate bool
		wantErr      bool
	}{
		{"public https", "https://example.com/hook", false, false},
		{"public http", "http://example.com/hook", false, false},
		{"bad scheme", "ftp://example.com/hook", false, true},
		{"no host", "https:///hook", false, true},
		{"credentials", "https://user:pass@example.com/hook", false, true},
		{"loopback ip", "http://127.0.0.1:8080/hook", false, true},
		{"loopback ip allowed in dev", "http://127.0.0.1:8080/hook", true, false},
		{"private ip", "http://10.0.0.5/hook", false, true},
		{"private 192.168", "http://192.168.1.10/hook", false, true},
		{"cloud metadata", "http://169.254.169.254/latest/meta-data/", false, true},
		{"unspecified", "http://0.0.0.0/hook", false, true},
		{"ipv6 loopback", "http://[::1]/hook", false, true},
		{"ipv6 ula", "http://[fd00::1]/hook", false, true},
		{"localhost name", "http://localhost:9999/hook", false, true},
		{"localhost name allowed in dev", "http://localhost:9999/hook", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateTargetURL(c.url, c.allowPrivate)
			if (err != nil) != c.wantErr {
				t.Errorf("ValidateTargetURL(%q, %v) err = %v, wantErr %v", c.url, c.allowPrivate, err, c.wantErr)
			}
		})
	}
}
