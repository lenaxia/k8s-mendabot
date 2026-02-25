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
		{
			name:  "finding 010: JWT bearer token uppercase",
			input: "Authorization: Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig",
			want:  "Authorization: Bearer [REDACTED]",
		},
		{
			name:  "finding 010: JWT bearer token lowercase",
			input: "bearer eyJhbGciOiJSUzI1NiJ9.payload.sig",
			want:  "bearer [REDACTED]",
		},
		{
			name:  "finding 011: JSON password no space",
			input: `{"username":"admin","password":"s3cr3t"}`,
			want:  `{"username":"admin","password":"[REDACTED]"}`,
		},
		{
			name:  "finding 011: JSON password with space after colon",
			input: `{"password": "hunter2"}`,
			want:  `{"password": "[REDACTED]"}`,
		},
		{
			name:  "finding 011: JSON password case-insensitive",
			input: `{"Password":"MySecret"}`,
			want:  `{"Password":"[REDACTED]"}`,
		},
		{
			name:  "finding 012: Redis URL with empty username",
			input: "redis://:s3cr3tpassword@redis.default.svc:6379/0",
			want:  "redis://[REDACTED]@redis.default.svc:6379/0",
		},
		{
			name:  "P-006: PEM RSA private key full block",
			input: "error: -----BEGIN RSA PRIVATE KEY-----\nMIIEow...\n-----END RSA PRIVATE KEY-----",
			want:  "error: [REDACTED-PEM-KEY]",
		},
		{
			name:  "P-006: PEM EC private key full block",
			input: "key: -----BEGIN EC PRIVATE KEY-----\nbase64data\n-----END EC PRIVATE KEY-----",
			want:  "key: [REDACTED-PEM-KEY]",
		},
		{
			name:  "P-006: PEM PRIVATE KEY (PKCS8) full block",
			input: "-----BEGIN PRIVATE KEY-----\nMIIEvQ==\n-----END PRIVATE KEY-----",
			want:  "[REDACTED-PEM-KEY]",
		},
		{
			name:  "P-006: PEM public key not redacted",
			input: "-----BEGIN PUBLIC KEY-----\nMFww...\n-----END PUBLIC KEY-----",
			want:  "-----BEGIN PUBLIC KEY-----\nMFww...\n-----END PUBLIC KEY-----",
		},
		{
			name:  "P-007: X-API-Key header",
			input: "X-API-Key: sk-abc123def456",
			want:  "X-API-Key: [REDACTED]",
		},
		{
			name:  "P-007: x-api-key header lowercase",
			input: "x-api-key: myshortkey",
			want:  "x-api-key: [REDACTED]",
		},
		{
			name:  "P-007: X-API-Key with tabs",
			input: "X-API-Key:\tmysecretvalue",
			want:  "X-API-Key:\t[REDACTED]",
		},
		{
			// Verifies the x-api-key pattern uses \s* (not \t*) so a space after
			// the separator is consumed and the value is caught by this pattern
			// (not silently falling through to the api-key pattern).
			name:  "P-007: X-API-Key space after colon caught by x-api-key pattern",
			input: "X-API-Key: mysecretvalue",
			want:  "X-API-Key: [REDACTED]",
		},
		{
			// gh[a-z]_ pattern fires before token= pattern, so the GH token gets
			// [REDACTED-GH-TOKEN]; then token= fires on "token: [REDACTED-GH-TOKEN]"
			// and replaces the value with [REDACTED]. Net result: token: [REDACTED].
			// Consistent: all token= contexts produce [REDACTED] regardless of value.
			name:  "GitHub App installation token ghs_ prefix in token= context",
			input: "token: ghs_16C7e42F292c6912E7710c838347Ae178B4a",
			want:  "token: [REDACTED]",
		},
		{
			name:  "GitHub Actions token gha_ prefix",
			input: "Authorization: gha_someTokenValue1234567890123456789012",
			want:  "Authorization: [REDACTED-GH-TOKEN]",
		},
		{
			name:  "GitHub PAT ghp_ prefix",
			input: "GITHUB_TOKEN=ghp_16C7e42F292c6912E7710c838347Ae178B4a",
			want:  "GITHUB_TOKEN=[REDACTED]",
		},
		{
			name:  "GitHub OAuth token gho_ prefix",
			input: "token gho_16C7e42F292c6912E7710c838347Ae178B4a",
			want:  "token [REDACTED-GH-TOKEN]",
		},
		{
			name:  "GitHub refresh token ghr_ prefix",
			input: "refresh_token=ghr_16C7e42F292c6912E7710c838347Ae178B4a",
			want:  "refresh_token=[REDACTED]",
		},
		// github_pat_ is GitHub's fine-grained PAT format. The gh[a-z]_ pattern
		// does not match it (prefix is too long). It IS caught by the token=
		// named-key pattern when a key is present, and its 59-char bech32 suffix
		// is caught by the base64 pattern (≥40 chars). A bare appearance without
		// any named key and with a short suffix would be missed — documented as
		// residual risk in P-010.
		{
			name:  "github_pat_ fine-grained PAT via token= key",
			input: "GITHUB_TOKEN=github_pat_11ABCDEFG0abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWX",
			want:  "GITHUB_TOKEN=[REDACTED]",
		},
		{
			name:  "github_pat_ fine-grained PAT suffix caught by base64 pattern",
			input: "github_pat_11ABCDEFG0abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQRSTUVWX",
			// The gh[a-z]_ pattern does not match (prefix too long). The underscore
			// breaks the base64 run, so only the alphanumeric suffix (≥40 chars) is
			// caught by the base64 pattern — the literal "github_pat_" prefix remains.
			// The secret payload is still redacted; this documents the residual prefix leak.
			want: "github_pat_[REDACTED-BASE64]",
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
