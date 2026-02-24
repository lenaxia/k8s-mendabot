// Package readiness provides a composable framework for checking that external
// dependencies (sink credentials, LLM endpoints) are available before the watcher
// is permitted to create RemediationJob objects.
//
// Usage:
//
//	sinkChecker  := readiness.NewCachedChecker(sink.NewGitHubAppChecker(...), 60*time.Second)
//	llmChecker   := readiness.NewCachedChecker(llm.NewOpenAIChecker(...), 60*time.Second)
//	combined     := readiness.All(sinkChecker, llmChecker)
//
//	if err := combined.Check(ctx); err != nil {
//	    // log error, requeue
//	}
package readiness

import (
	"context"
	"errors"
)

// Checker is the interface implemented by all readiness probes.
// Implementations must be safe for concurrent use.
type Checker interface {
	// Name returns a stable, human-readable identifier for this checker
	// (e.g. "github-app", "llm/openai"). Used in log messages.
	Name() string

	// Check performs the readiness probe. It returns nil when the dependency
	// is available and a descriptive error otherwise.
	Check(ctx context.Context) error
}

// allChecker runs every sub-checker in order and joins all errors.
type allChecker struct {
	checkers []Checker
}

// All returns a Checker that passes only when every provided checker passes.
// Errors from all failing checkers are joined and returned together so the
// caller gets a complete picture in a single log line.
// If ctx is cancelled between checker invocations, the loop stops immediately
// and the cancellation error is appended to any already-collected errors.
func All(checkers ...Checker) Checker {
	return &allChecker{checkers: checkers}
}

func (a *allChecker) Name() string { return "all" }

func (a *allChecker) Check(ctx context.Context) error {
	var errs []error
	for _, c := range a.checkers {
		if ctx.Err() != nil {
			errs = append(errs, ctx.Err())
			break
		}
		if err := c.Check(ctx); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// NopChecker always passes. Use it as the LLM or sink checker when no provider
// is configured so the gate is not inadvertently triggered by an unconfigured
// optional dependency.
type NopChecker struct {
	name string
}

// NewNopChecker returns a *NopChecker with the given name.
func NewNopChecker(name string) *NopChecker {
	return &NopChecker{name: name}
}

func (n *NopChecker) Name() string                  { return n.name }
func (n *NopChecker) Check(_ context.Context) error { return nil }
