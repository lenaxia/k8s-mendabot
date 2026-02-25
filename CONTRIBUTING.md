# Contributing to k8s-mendabot

Thank you for your interest in contributing. This document covers how to work
within this project effectively and what is expected of contributors.

---

## Table of Contents

1. [Code of Conduct](#code-of-conduct)
2. [Getting Started](#getting-started)
3. [Development Setup](#development-setup)
4. [Branching and Workflow](#branching-and-workflow)
5. [Coding Standards](#coding-standards)
6. [Testing Requirements](#testing-requirements)
7. [Commit Messages](#commit-messages)
8. [Pull Requests](#pull-requests)
9. [Worklogs](#worklogs)
10. [Reporting Issues](#reporting-issues)

---

## Code of Conduct

This project follows the [Contributor Covenant Code of Conduct](CODE_OF_CONDUCT.md).
By participating you agree to uphold it.

---

## Getting Started

### Prerequisites

- Go 1.24+
- Docker (for agent image builds)
- `kubectl` with access to a Kubernetes cluster (>= 1.28) for integration testing
- [`golangci-lint`](https://golangci-lint.run/usage/install/)
- [`gitleaks`](https://github.com/zricethezav/gitleaks)

Install linting tools:

```sh
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/zricethezav/gitleaks/v8@latest
```

### Install git hooks

After cloning, run once:

```sh
make install-hooks
```

This installs a pre-commit hook that runs `gitleaks` (secrets scan) and
`golangci-lint` on every commit. Do not bypass with `--no-verify` except in
genuine emergencies.

---

## Development Setup

```sh
# Clone the repository
git clone git@github.com:lenaxia/k8s-mendabot.git
cd k8s-mendabot

# Tidy dependencies
go mod tidy

# Build the watcher binary
go build -o bin/mendabot-watcher ./cmd/watcher/

# Run all tests
make test
```

### Useful make targets

| Target | Description |
|---|---|
| `make lint` | Quick `go vet` check |
| `make lint-full` | Full `golangci-lint` run |
| `make lint-secrets` | Full repo secrets scan |
| `make lint-security` | `gosec` HIGH/CRITICAL security check |
| `make test` | Full test suite with race detector |
| `make install-hooks` | (Re-)install git hooks |

---

## Branching and Workflow

All work happens on a feature branch; nothing is committed directly to `main`.

**Branch naming:**

| Type | Pattern | Example |
|---|---|---|
| Feature | `feature/<short-description>` | `feature/epic30-dry-run-mode` |
| Bug fix | `bugfix/<issue-description>` | `bugfix/fingerprint-collision` |
| Hotfix | `hotfix/<critical-issue>` | `hotfix/token-expiry-crash` |

**Workflow:**

1. Create a branch from `main`
2. Update the active-branches table in `README-LLM.md`
3. Work in the branch with regular, logical commits
4. Ensure all tests pass: `make test`
5. Write a worklog entry (see [Worklogs](#worklogs))
6. Open a pull request against `main`
7. Address review feedback
8. Merge and update `README-LLM.md` branch tables

---

## Coding Standards

This project follows the rules defined in [`README-LLM.md`](README-LLM.md).
The key points are:

**Type safety**
- Define strongly-typed structs for all data structures.
- Never use `map[string]interface{}` for structured data. If you must parse
  unknown external JSON/YAML, convert to a typed struct immediately.

**Idiomatic Go**
- Use the `(value, error)` multiple-return pattern.
- Avoid global state.
- Create custom error types for domain-specific errors.
- Prefer minimal concurrency; add it only when there is a clear, measurable
  benefit.

**Explicit over implicit**
- Handle every error — never swallow them.
- Use explicit type declarations.
- No magic or hidden behaviour.

**Code quality**
- Write no comments unless they are strictly necessary and timeless.
- Remove or correct any incorrect or outdated comments.
- Code is self-documenting through clear naming.

**Zero technical debt**
- Do not create adapters for backwards compatibility.
- Remove legacy code.
- Implement the full final solution.
- Never hack tests to pass — fix the root cause.

---

## Testing Requirements

This project follows strict Test-Driven Development (TDD).

**Workflow:**

1. Write the test first.
2. Run it — it must fail.
3. Write the minimal code to make it pass.
4. Run it again — it must pass.
5. Refactor if needed.

**Coverage:**

- Multiple happy-path cases.
- Multiple unhappy-path cases.
- Edge cases (empty fields, nil slices, very long strings, etc.).
- Error conditions.

**Always use `-timeout`:**

```sh
# Good
go test -timeout 30s -race ./...

# Bad — can hang forever
go test ./...
```

**envtest integration tests** in `internal/controller/` have additional rules
— read the relevant section in [`README-LLM.md`](README-LLM.md#testing-requirements)
before touching that package.

---

## Commit Messages

Use the conventional commits format:

```
<type>(<scope>): <short summary>
```

Common types: `feat`, `fix`, `docs`, `refactor`, `test`, `chore`, `ci`.

Examples:

```
feat(provider): add PVC provisioning failure detection
fix(controller): prevent duplicate RemediationJob creation under concurrent reconciles
docs(readme): add namespace-level annotation control examples
test(jobbuilder): add edge-case coverage for empty finding errors
```

- Keep the summary line at or below 72 characters.
- Use the imperative mood: "add", "fix", "remove" — not "added", "fixed".
- Reference a GitHub issue or PR number in the body when applicable.

---

## Pull Requests

**Before opening a PR:**

- All tests pass: `make test`
- Full lint passes: `make lint-full`
- No secrets detected: `make lint-secrets`
- A worklog entry has been written.

**PR body must include:**

1. What the change does and why.
2. How it was tested (test commands and outcomes).
3. Any follow-up items or known limitations.

**Review:**

- At least one approving review is required before merging.
- Address all reviewer comments before merging.
- Squash-merge or rebase are both acceptable; preserve a clean, readable
  history.

---

## Worklogs

Worklogs are mandatory for any meaningful session. They are the institutional
memory of this project.

Write a worklog entry after completing a user story, making an architectural
decision, discovering a bug, or any session longer than 30 minutes.

Format and naming rules are defined in full in
[`README-LLM.md`](README-LLM.md#worklog-requirements). Summary:

- File: `docs/WORKLOGS/NNNN_YYYY-MM-DD_short-description.md`
- Sections: Objective, Work Completed, Key Decisions, Blockers, Tests Run,
  Next Steps, Files Modified.
- Write it before ending the session — not the next day.
- Worklogs are append-only history; never retroactively rewrite them.

---

## Reporting Issues

- Search existing issues before opening a new one.
- For security vulnerabilities, follow the [Security Policy](SECURITY.md) —
  do **not** open a public issue.
- For all other bugs and feature requests, open a GitHub issue with:
  - A clear, specific title.
  - Steps to reproduce (for bugs).
  - Expected vs. actual behaviour.
  - Relevant log output, Kubernetes events, or `kubectl describe` output.
  - Cluster version, Helm chart version, and `agentType` configuration.
