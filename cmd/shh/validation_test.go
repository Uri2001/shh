package main

import "testing"

func TestNormalizeHost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "trimmed host",
			input: " example.com ",
			want:  "example.com",
		},
		{
			name:  "alias",
			input: "my-alias",
			want:  "my-alias",
		},
		{
			name:  "ipv6",
			input: "[2001:db8::1]",
			want:  "[2001:db8::1]",
		},
		{
			name:    "empty",
			input:   "   ",
			wantErr: true,
		},
		{
			name:    "spaces inside",
			input:   "bad host",
			wantErr: true,
		},
		{
			name:    "shell injection",
			input:   "localhost;rm -rf /",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeHost(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeHost(%q) expected error", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeHost(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
