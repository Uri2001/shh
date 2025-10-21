package main

import "testing"

func TestParseHistoryLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		line string
		host string
		ok   bool
	}{
		{
			name: "simple host",
			line: "ssh example.com",
			host: "example.com",
			ok:   true,
		},
		{
			name: "with user and port",
			line: "ssh -p 2222 user@example.com",
			host: "example.com",
			ok:   true,
		},
		{
			name: "trailing command",
			line: "ssh host.local uptime",
			host: "host.local",
			ok:   true,
		},
		{
			name: "zsh history format",
			line: ": 1700000000:0;ssh -i ~/.ssh/id_ed25519 git@github.com",
			host: "github.com",
			ok:   true,
		},
		{
			name: "config alias",
			line: "ssh my-alias",
			host: "my-alias",
			ok:   true,
		},
		{
			name: "option with value",
			line: "ssh -F ~/.ssh/config work-host",
			host: "work-host",
			ok:   true,
		},
		{
			name: "invalid command",
			line: "git push origin main",
			ok:   false,
		},
		{
			name: "no host provided",
			line: "ssh --help",
			ok:   false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, ok := parseHistoryLine(tc.line)
			if ok != tc.ok {
				t.Fatalf("parseHistoryLine(%q) ok=%v, want %v", tc.line, ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if got != tc.host {
				t.Fatalf("parseHistoryLine(%q) host=%q, want %q", tc.line, got, tc.host)
			}
		})
	}
}
