package main

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	previewv1 "github.com/connoralydon/work-preview/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func TestRandomHexID(t *testing.T) {
	id := randomHexID()
	if len(id) != 12 {
		t.Fatalf("randomHexID() length=%d, want 12", len(id))
	}
	if _, err := hex.DecodeString(id); err != nil {
		t.Fatalf("randomHexID()=%q is not hexadecimal: %v", id, err)
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

func TestWritePreviewListAlignsHeaders(t *testing.T) {
	var output bytes.Buffer
	expiresAt := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	previews := []*previewv1.Preview{{
		Id: "abcdefghijklmnopqrst", Url: "https://example.p.boringbison.xyz", Port: 3000,
		ExpiresAt: timestamppb.New(expiresAt),
	}}

	if err := writePreviewList(&output, previews); err != nil {
		t.Fatal(err)
	}
	want := "ID                    URL                                PORT  EXPIRES AT\n" +
		"abcdefghijklmnopqrst  https://example.p.boringbison.xyz  3000  2026-07-13T12:00:00Z\n"
	if output.String() != want {
		t.Fatalf("writePreviewList() output:\n%q\nwant:\n%q", output.String(), want)
	}
}
