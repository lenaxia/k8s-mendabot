package domain_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"
	"testing"

	"github.com/lenaxia/k8s-mendabot/internal/domain"
)

// TestFinding_ZeroValue verifies the zero value of Finding is safe (no panic, zero values).
func TestFinding_ZeroValue(t *testing.T) {
	var f domain.Finding
	if f.Kind != "" {
		t.Errorf("zero value Finding.Kind should be empty, got %q", f.Kind)
	}
	if f.Errors != "" {
		t.Errorf("zero value Finding.Errors should be empty, got %q", f.Errors)
	}
}

// TestFinding_EmptyParentObject verifies that a Finding with an empty ParentObject
// is safe to construct and access — no panic, zero value behaviour.
func TestFinding_EmptyParentObject(t *testing.T) {
	f := domain.Finding{
		Kind:         "Pod",
		Name:         "orphan-pod",
		Namespace:    "default",
		ParentObject: "",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
	}
	if f.ParentObject != "" {
		t.Errorf("expected empty ParentObject, got %q", f.ParentObject)
	}
}

// TestFinding_EmptyErrors verifies that a Finding with an empty Errors string
// is safe to construct and access — no panic, zero value behaviour.
func TestFinding_EmptyErrors(t *testing.T) {
	f := domain.Finding{
		Kind:      "Deployment",
		Name:      "my-deploy",
		Namespace: "default",
		Errors:    "",
	}
	if f.Errors != "" {
		t.Errorf("expected empty Errors, got %q", f.Errors)
	}
}

// TestFindingFingerprint covers all acceptance-criteria cases from STORY_01.
func TestFindingFingerprint(t *testing.T) {
	base := &domain.Finding{
		Kind:         "Pod",
		Name:         "pod-abc-1",
		Namespace:    "default",
		ParentObject: "Deployment/my-app",
		Errors:       `[{"text":"CrashLoopBackOff"}]`,
	}

	t.Run("Deterministic", func(t *testing.T) {
		fp1, err1 := domain.FindingFingerprint(base)
		fp2, err2 := domain.FindingFingerprint(base)
		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected error: %v %v", err1, err2)
		}
		if fp1 != fp2 {
			t.Errorf("non-deterministic: %q != %q", fp1, fp2)
		}
	})

	t.Run("ErrorOrderIndependent", func(t *testing.T) {
		f1 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"b"},{"text":"a"}]`,
		}
		f2 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"a"},{"text":"b"}]`,
		}
		fp1, err1 := domain.FindingFingerprint(f1)
		fp2, err2 := domain.FindingFingerprint(f2)
		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected error: %v %v", err1, err2)
		}
		if fp1 != fp2 {
			t.Errorf("error order should not affect fingerprint: %q != %q", fp1, fp2)
		}
	})

	t.Run("SameParentDifferentNames", func(t *testing.T) {
		f1 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Name: "pod-abc-1", Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		f2 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Name: "pod-abc-2", Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		fp1, err1 := domain.FindingFingerprint(f1)
		fp2, err2 := domain.FindingFingerprint(f2)
		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected error: %v %v", err1, err2)
		}
		if fp1 != fp2 {
			t.Errorf("Name should not affect fingerprint: %q != %q", fp1, fp2)
		}
	})

	t.Run("DifferentErrors", func(t *testing.T) {
		f1 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		f2 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"ImagePullBackOff"}]`,
		}
		fp1, err1 := domain.FindingFingerprint(f1)
		fp2, err2 := domain.FindingFingerprint(f2)
		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected error: %v %v", err1, err2)
		}
		if fp1 == fp2 {
			t.Errorf("different errors must produce different fingerprints: both %q", fp1)
		}
	})

	t.Run("DifferentParents", func(t *testing.T) {
		f1 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/deploy-a",
			Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		f2 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/deploy-b",
			Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		fp1, _ := domain.FindingFingerprint(f1)
		fp2, _ := domain.FindingFingerprint(f2)
		if fp1 == fp2 {
			t.Errorf("different parents must produce different fingerprints: both %q", fp1)
		}
	})

	t.Run("DifferentNamespaces", func(t *testing.T) {
		f1 := &domain.Finding{
			Kind: "Pod", Namespace: "ns-a", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		f2 := &domain.Finding{
			Kind: "Pod", Namespace: "ns-b", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		fp1, _ := domain.FindingFingerprint(f1)
		fp2, _ := domain.FindingFingerprint(f2)
		if fp1 == fp2 {
			t.Errorf("different namespaces must produce different fingerprints: both %q", fp1)
		}
	})

	t.Run("DifferentKinds", func(t *testing.T) {
		f1 := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		f2 := &domain.Finding{
			Kind: "Deployment", Namespace: "default", ParentObject: "Deployment/my-app",
			Errors: `[{"text":"CrashLoopBackOff"}]`,
		}
		fp1, _ := domain.FindingFingerprint(f1)
		fp2, _ := domain.FindingFingerprint(f2)
		if fp1 == fp2 {
			t.Errorf("different kinds must produce different fingerprints: both %q", fp1)
		}
	})

	t.Run("EmptyErrors", func(t *testing.T) {
		f1 := &domain.Finding{Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app", Errors: ""}
		f2 := &domain.Finding{Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app", Errors: "[]"}
		fp1, err1 := domain.FindingFingerprint(f1)
		fp2, err2 := domain.FindingFingerprint(f2)
		if err1 != nil || err2 != nil {
			t.Fatalf("unexpected error: %v %v", err1, err2)
		}
		if fp1 != fp2 {
			t.Errorf("empty string and '[]' errors should produce same fingerprint: %q != %q", fp1, fp2)
		}
	})

	t.Run("HTMLCharacters", func(t *testing.T) {
		f := &domain.Finding{
			Kind:         "Pod",
			Namespace:    "default",
			ParentObject: "Deployment/my-app",
			Errors:       `[{"text":"error: <html> & more > less"}]`,
		}

		actualFP, err := domain.FindingFingerprint(f)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if actualFP == "" {
			t.Fatal("fingerprint must not be empty")
		}

		// Helper: fingerprint using the same payload struct as FindingFingerprint,
		// but with escapeHTML toggled.
		fingerprintWith := func(escapeHTML bool) string {
			var failures []struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal([]byte(f.Errors), &failures)
			texts := make([]string, 0, len(failures))
			for _, fv := range failures {
				texts = append(texts, fv.Text)
			}
			sort.Strings(texts)

			payload := struct {
				Namespace    string   `json:"namespace"`
				Kind         string   `json:"kind"`
				ParentObject string   `json:"parentObject"`
				ErrorTexts   []string `json:"errorTexts"`
			}{
				Namespace:    f.Namespace,
				Kind:         f.Kind,
				ParentObject: f.ParentObject,
				ErrorTexts:   texts,
			}

			var buf bytes.Buffer
			enc := json.NewEncoder(&buf)
			enc.SetEscapeHTML(escapeHTML)
			_ = enc.Encode(payload)
			return fmt.Sprintf("%x", sha256.Sum256(buf.Bytes()))
		}

		// The actual fingerprint must match what you get with SetEscapeHTML(false).
		expectedNoEscape := fingerprintWith(false)
		if actualFP != expectedNoEscape {
			t.Errorf("FindingFingerprint does not match SetEscapeHTML(false) encoding:\n  got  %q\n  want %q", actualFP, expectedNoEscape)
		}

		// The HTML-escaped encoding must produce a DIFFERENT fingerprint,
		// proving SetEscapeHTML(false) is load-bearing.
		escapedFP := fingerprintWith(true)
		if actualFP == escapedFP {
			t.Errorf("SetEscapeHTML(true) produced same fingerprint as SetEscapeHTML(false) — HTML characters in error text must cause a difference")
		}
	})

	t.Run("Returns64HexChars", func(t *testing.T) {
		fp, err := domain.FindingFingerprint(base)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(fp) != 64 {
			t.Errorf("fingerprint must be 64 hex chars, got %d: %q", len(fp), fp)
		}
		for _, c := range fp {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("fingerprint contains non-lowercase-hex char %q: %q", c, fp)
			}
		}
	})

	t.Run("InvalidErrorsJSON", func(t *testing.T) {
		f := &domain.Finding{
			Kind: "Pod", Namespace: "default", ParentObject: "Deployment/my-app",
			Errors: `not-valid-json`,
		}
		_, err := domain.FindingFingerprint(f)
		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})
}
