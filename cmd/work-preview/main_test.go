package main

import (
	"strings"
	"testing"
)

func TestFormatGitPrefix(t *testing.T) {
	tests := []struct {
		name       string
		commit     string
		branch     string
		repository string
		want       string
	}{
		{
			name: "common values", commit: "A1B2C3D4", branch: "main", repository: "work-preview",
			want: "a1b2c3d4-main-work-preview",
		},
		{
			name: "dns sanitization", commit: "abc123", branch: "feature/Login_Page", repository: "Work Preview",
			want: "abc123-feature-login-page-work-preview",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatGitPrefix(test.commit, test.branch, test.repository); got != test.want {
				t.Fatalf("formatGitPrefix()=%q, want %q", got, test.want)
			}
		})
	}
}

func TestFormatGitPrefixFitsDNSLabel(t *testing.T) {
	prefix := formatGitPrefix("1234567890abcdef", strings.Repeat("long-branch-", 10), strings.Repeat("repository-", 5))
	if len(prefix) > 63 {
		t.Fatalf("prefix length=%d, want at most 63: %q", len(prefix), prefix)
	}
	if strings.HasPrefix(prefix, "-") || strings.HasSuffix(prefix, "-") {
		t.Fatalf("invalid DNS label: %q", prefix)
	}
}
