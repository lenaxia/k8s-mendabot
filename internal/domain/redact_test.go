package domain_test

import (
	"testing"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

func TestRedactSecrets(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "URL credentials postgres",
			input: "failed to connect to postgres://myuser:s3cr3tpass@db.example.com:5432/mydb",
			want:  "failed to connect to postgres://[REDACTED]@db.example.com:5432/mydb",
		},
		{
			name:  "URL credentials https",
			input: "clone failed: https://user:ghp_token123@github.com/org/repo.git",
			want:  "clone failed: https://[REDACTED]@github.com/org/repo.git",
		},
		{
			name:  "password= assignment",
			input: "DATABASE_PASSWORD=hunter2 caused startup failure",
			want:  "DATABASE_PASSWORD=[REDACTED] caused startup failure",
		},
		{
			name:  "password: assignment with colon",
			input: "password: s3cr3t value here",
			want:  "password: [REDACTED] value here",
		},
		{
			name:  "token= assignment",
			input: "GITHUB_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz012345 rejected",
			want:  "GITHUB_TOKEN=[REDACTED] rejected",
		},
		{
			name:  "secret= assignment",
			input: "client_secret=abc123def456ghi789 auth failed",
			want:  "client_secret=[REDACTED] auth failed",
		},
		{
			name:  "api-key= assignment",
			input: "api-key=sk-abc123xyz456longkeyvalue00000000",
			want:  "api-key=[REDACTED]",
		},
		{
			name:  "api_key= assignment",
			input: "api_key=sk-abc123xyz456longkeyvalue00000000",
			want:  "api_key=[REDACTED]",
		},
		{
			name:  "apikey= assignment",
			input: "apikey=sk-abc123xyz456longkeyvalue00000000",
			want:  "apikey=[REDACTED]",
		},
		{
			name:  "base64 string exactly 40 chars",
			input: "value: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			want:  "value: [REDACTED-BASE64]",
		},
		{
			name:  "base64 string longer than 40 chars",
			input: "value: c2VjcmV0dmFsdWV0aGF0aXNsb25nZW5vdWdodG9iZXJlZGFjdGVk",
			want:  "value: [REDACTED-BASE64]",
		},
		{
			name:  "base64 string less than 40 chars not redacted",
			input: "short: YWJjZGVmZ2g=",
			want:  "short: YWJjZGVmZ2g=",
		},
		{
			name:  "base64 string 39 chars not redacted",
			input: "short: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
			want:  "short: AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		},
		{
			name:  "clean text unchanged",
			input: "container app: CrashLoopBackOff (last exit: OOMKilled)",
			want:  "container app: CrashLoopBackOff (last exit: OOMKilled)",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "multiple patterns in one string",
			input: "user=admin password=s3cr3t token=abc123",
			want:  "user=admin password=[REDACTED] token=[REDACTED]",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := domain.RedactSecrets(tt.input)
			if got != tt.want {
				t.Errorf("RedactSecrets(%q)\n  got  %q\n  want %q", tt.input, got, tt.want)
			}
		})
	}
}
