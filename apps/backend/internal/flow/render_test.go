package flow

import "testing"

func TestRenderText(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		vars map[string]string
		want string
	}{
		{
			name: "no-space token",
			tmpl: "Hi {{name}}!",
			vars: map[string]string{"name": "Ada"},
			want: "Hi Ada!",
		},
		{
			name: "spaced token",
			tmpl: "Hi {{ name }}!",
			vars: map[string]string{"name": "Ada"},
			want: "Hi Ada!",
		},
		{
			name: "both spacings of the same key in one template",
			tmpl: "{{name}} and {{ name }}",
			vars: map[string]string{"name": "Ada"},
			want: "Ada and Ada",
		},
		{
			name: "multiple distinct tokens",
			tmpl: "{{greeting}}, {{ name }} - see {{product.url}}",
			vars: map[string]string{"greeting": "Hello", "name": "Ada", "product.url": "https://x/p"},
			want: "Hello, Ada - see https://x/p",
		},
		{
			name: "repeated token substituted everywhere",
			tmpl: "{{x}}{{x}}{{x}}",
			vars: map[string]string{"x": "ab"},
			want: "ababab",
		},
		{
			name: "unknown tags left untouched",
			tmpl: "{{known}} but {{unknown}} and {{ also.missing }}",
			vars: map[string]string{"known": "Y"},
			want: "Y but {{unknown}} and {{ also.missing }}",
		},
		{
			name: "empty vars map leaves template unchanged",
			tmpl: "Hi {{name}}, welcome {{ name }}",
			vars: map[string]string{},
			want: "Hi {{name}}, welcome {{ name }}",
		},
		{
			name: "nil vars map leaves template unchanged",
			tmpl: "Hi {{name}}",
			vars: nil,
			want: "Hi {{name}}",
		},
		{
			name: "empty template",
			tmpl: "",
			vars: map[string]string{"name": "Ada"},
			want: "",
		},
		{
			name: "value substituted with empty string",
			tmpl: "x{{gap}}y",
			vars: map[string]string{"gap": ""},
			want: "xy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := renderText(tt.tmpl, tt.vars); got != tt.want {
				t.Errorf("renderText(%q, %v) = %q, want %q", tt.tmpl, tt.vars, got, tt.want)
			}
		})
	}
}
