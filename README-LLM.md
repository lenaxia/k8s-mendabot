# k8s-mendabot — LLM Implementation Guide

**Version:** 1.0
**Last Updated:** 2026-02-19
**Project Status:** Active Development — Design Phase

---

## Table of Contents

1. [Project Overview](#project-overview)
2. [Critical Guidelines & Hard Rules](#critical-guidelines--hard-rules)
3. [Repository Structure](#repository-structure)
4. [Architecture Overview](#architecture-overview)
5. [Technology Stack](#technology-stack)
6. [Worklog Requirements](#worklog-requirements)
7. [Development Workflow](#development-workflow)
8. [Multi-Agent Workflow](#multi-agent-workflow)
9. [Common Commands](#common-commands)
10. [Branch Management](#branch-management)
11. [Testing Requirements](#testing-requirements)

---

## Project Overview

**k8s-mendabot** watches core Kubernetes resources (Pods, Deployments, StatefulSets,
PersistentVolumeClaims, Nodes, Jobs) directly, deduplicates findings by parent resource,
and spawns a per-finding Kubernetes Job that runs an
[OpenCode](https://opencode.ai) agent in-cluster. The agent investigates the live cluster
and the GitOps repository, then opens a pull request with a proposed fix.

**Core principles:**
- One PR per unique finding, deduplicated by parent resource + error fingerprint
- Strictly read-only cluster access for the investigation agent
- No direct commits to the GitOps repo's default branch — PRs only
- CRD-based deduplication state via `RemediationJob` objects (survives restarts, no external store)
- Self-contained Kubernetes deployment via Kustomize, compatible with Flux GitOps

**Two deliverables:**
1. `mendabot-watcher` — Go controller binary (controller-runtime)
2. `mendabot-agent` — Docker image (opencode + kubectl + helm + flux + gh)

**Note:** Upstream contribution feature has been removed due to GitHub App permission complexity. The system focuses on self-remediation cascade prevention without attempting upstream bug reporting.

**Primary source documents:**
- [`docs/DESIGN/HLD.md`](docs/DESIGN/HLD.md) — Authoritative specification
- [`docs/DESIGN/lld/`](docs/DESIGN/lld/) — Low-level designs (9 LLDs)
- [`docs/BACKLOG/`](docs/BACKLOG/) — Epics and user stories
- [`docs/WORKLOGS/`](docs/WORKLOGS/) — Session worklogs

---

## Critical Guidelines & Hard Rules

### 0. Test Driven Development (TDD)

**MANDATORY:** Write tests BEFORE writing functional code. Always.

```
Correct workflow:
1. Write test
2. Run test (must fail)
3. Write minimal code to pass
4. Run test (must pass)
5. Refactor if needed
```

**Test requirements:**
- Multiple happy path tests
- Multiple unhappy path tests
- Edge case coverage
- Always use `-timeout` when running tests
- Tests must pass before marking work complete

### 1. Type Safety First

**Always:**
- Define strongly-typed structs for all data structures
- Create domain types for related fields

**Never:**
- Use `map[string]interface{}` for structured data
- Use `interface{}` when the type is known
- Pass untyped data between functions

Maps are acceptable only when parsing external JSON/YAML with unknown structure —
and even then, convert to a typed struct immediately.

### 2. Idiomatic Go

- Follow Go conventions throughout
- Use `(value, error)` multiple return pattern
- Avoid global state
- Create custom error types for domain-specific errors
- Prefer minimal concurrency; add it only when there is clear, measurable benefit

### 3. Explicit Over Implicit

- Explicit error handling — no swallowed errors
- Explicit type declarations
- No magic or hidden behaviour

### 4. Code Quality

- No comments unless strictly necessary and timeless
- Incorrect or outdated comments must be removed or corrected
- Code is self-documenting through clear naming

### 5. Zero Technical Debt

- Do not create adapters for backwards compatibility
- Remove legacy code
- Implement the full final solution
- Never hack tests to pass — fix the root cause

### 6. Uncertainty Protocol

If uncertain about correct behaviour: **ask the user**. Do not guess, assume, or implement
workarounds.

### 7. Understand the Architecture First

Before making any change, read the HLD and the relevant LLD. Understand how the change
fits the overall data flow. Never modify code without knowing why.

### 8. Communication Tone

- Neutral, factual, objective
- Not sensational or sycophantic
- Provide honest and critical feedback
- Validate claims with evidence before stating them

---

## Repository Structure

```
k8s-mendabot/
├── README.md                          # User-facing README
├── README-LLM.md                      # This file
├── go.mod
├── go.sum
│
├── api/
│   └── v1alpha1/
│       ├── result_types.go            # Vendored k8sgpt Result + Failure + Sensitive types
│       └── remediationjob_types.go    # RemediationJob CRD types + deep copy + AddToScheme
│
├── cmd/
│   └── watcher/
│       └── main.go                    # Scheme registration, provider loop, manager start
│
├── internal/
│   ├── config/
│   │   ├── config.go                  # Config struct + FromEnv()
│   │   └── config_test.go
│   ├── domain/
│   │   ├── interfaces.go              # JobBuilder interface
│   │   └── provider.go                # SourceProvider interface + Finding + SourceRef types
│   ├── provider/
│   │   ├── provider.go                # SourceProviderReconciler (generic, wraps any SourceProvider)
│   │   └── k8sgpt/
│   │       ├── provider.go            # K8sGPTProvider — implements SourceProvider
│   │       ├── provider_test.go
│   │       ├── reconciler.go          # ResultReconciler (concrete ctrl.Reconciler, internal detail)
│   │       └── reconciler_test.go
│   ├── controller/
│   │   ├── remediationjob_controller.go
│   │   ├── remediationjob_controller_test.go
│   │   └── suite_test.go              # envtest bootstrap
│   ├── jobbuilder/
│   │   ├── job.go                     # Builder struct + Build() method
│   │   └── job_test.go
│   └── logging/
│       └── logging.go                 # Zap logger construction
│
├── deploy/
│   └── kustomize/
│       ├── kustomization.yaml
│       ├── namespace.yaml
│       ├── crd-remediationjob.yaml
│       ├── serviceaccount-watcher.yaml
│       ├── serviceaccount-agent.yaml
│       ├── clusterrole-watcher.yaml
│       ├── clusterrole-agent.yaml
│       ├── clusterrolebinding-watcher.yaml
│       ├── clusterrolebinding-agent.yaml
│       ├── role-watcher.yaml
│       ├── rolebinding-watcher.yaml
│       ├── role-agent.yaml
│       ├── rolebinding-agent.yaml
│       ├── configmap-prompt.yaml
│       ├── secret-github-app.yaml     # Placeholder — fill manually, never commit real values
│       ├── secret-llm.yaml            # Placeholder — fill manually, never commit real values
│       └── deployment-watcher.yaml
│
├── docker/
│   ├── Dockerfile.agent               # debian-slim + opencode + kubectl + k8sgpt + helm + flux + gh
│   ├── Dockerfile.watcher             # multi-stage Go build → debian-slim runtime
│   └── scripts/
│       ├── get-github-app-token.sh    # Exchanges GitHub App private key for installation token
│       └── agent-entrypoint.sh        # envsubst prompt + opencode run --file
│
├── docs/
│   ├── README.md
│   ├── DESIGN/
│   │   ├── HLD.md                     # Authoritative high-level design
│   │   └── lld/
│   │       ├── CONTROLLER_LLD.md
│   │       ├── JOBBUILDER_LLD.md
│   │       ├── REMEDIATIONJOB_LLD.md
│   │       ├── PROVIDER_LLD.md
│   │       ├── SINK_PROVIDER_LLD.md
│   │       ├── AGENT_IMAGE_LLD.md
│   │       ├── WATCHER_IMAGE_LLD.md
│   │       ├── DEPLOY_LLD.md
│   │       └── PROMPT_LLD.md
│   ├── BACKLOG/
│   │   ├── README.md
│   │   ├── epic00-foundation/
│   │   ├── epic00.1-interfaces/
│   │   ├── epic01-controller/
│   │   ├── epic02-jobbuilder/
│   │   ├── epic03-agent-image/
│   │   ├── epic04-deploy/
│   │   ├── epic05-prompt/
│   │   ├── epic06-ci-cd/
│   │   ├── epic11-self-remediation-cascade/
│   │   ├── epic12-security-review/
│   │   └── epic13-multi-signal-correlation/
│   └── WORKLOGS/
│       └── README.md
│
└── .github/
    └── workflows/
        ├── build-watcher.yaml         # Builds and pushes watcher image to ghcr.io
        ├── build-agent.yaml           # Builds and pushes agent image to ghcr.io
        └── test.yaml                  # go test ./...
```

**Key principles:**
- Every major folder has a README.md
- READMEs are the first thing to read when entering a folder
- READMEs are short but define rules for reading and editing

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│  Kubernetes Cluster                                                  │
│                                                                      │
│  ┌──────────────────┐  writes   ┌──────────────────────────────┐   │
│  │  k8sgpt-operator │ ────────▶ │  Result CRDs                 │   │
│  │  (pre-existing)  │           │  (results.core.k8sgpt.ai)    │   │
│  └──────────────────┘           └──────────────┬───────────────┘   │
│                                                 │ watch             │
│                                  ┌──────────────▼───────────────┐  │
│                                  │  mendabot-watcher             │  │
│                                  │  (Deployment)                 │  │
│                                  │                               │  │
│                                  │  SourceProviderReconciler     │  │
│                                  │  + K8sGPTProvider             │  │
│                                  │  - watches Result CRDs        │  │
│                                  │  - creates RemediationJob CRDs│  │
│                                  │                               │  │
│                                  │  RemediationJobReconciler     │  │
│                                  │  - watches RemediationJob CRDs│  │
│                                  │  - creates batch/v1 Jobs      │  │
│                                  │  - syncs Job status back      │  │
│                                  └──────────────┬───────────────┘  │
│                                                 │ creates           │
│                              ┌──────────────────▼──────────────┐   │
│                              │  RemediationJob CRDs            │   │
│                              │  (remediation.mendabot.io)      │   │
│                              │  - durable dedup state          │   │
│                              │  - survives watcher restarts    │   │
│                              └──────────────────┬──────────────┘   │
│                                                 │ creates           │
│                                  ┌──────────────▼───────────────┐  │
│                                  │  mendabot-agent Job           │  │
│                                  │  (one per unique finding)     │  │
│                                  │                               │  │
│                                  │  init: git clone GitOps repo  │  │
│                                  │  main: opencode run <prompt>  │  │
│                                  │    tools: kubectl (read-only) │  │
│                                  │           k8sgpt analyze      │  │
│                                  │           gh pr create        │  │
│                                  └───────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────┘
                                          │
                                          ▼ opens PR
                         ┌────────────────────────────────┐
                         │  lenaxia/talos-ops-prod         │
                         │  (GitOps repo)                  │
                         └────────────────────────────────┘
```

### Deduplication logic

Deduplication is performed via the Kubernetes API using `RemediationJob` CRDs as durable
state, keyed by a **parent-resource fingerprint**:

```
fingerprint = sha256( namespace + kind + parentObject + sorted(error[].text) )
```

Using `parentObject` (e.g. the owning Deployment) rather than the individual resource name
means repeated pod restarts from the same bad Deployment produce one investigation, not one
per pod. If the error set changes materially (hash changes), a new investigation is triggered.

State is stored in `RemediationJob` objects in etcd — it survives watcher restarts. On
restart, Result CRDs re-reconcile; the `SourceProviderReconciler` lists existing `RemediationJob`
objects and skips any with a non-Failed phase. No in-memory map is used.

### Job lifecycle

| Setting | Value |
|---|---|
| `restartPolicy` | `Never` |
| `backoffLimit` | `1` |
| `activeDeadlineSeconds` | `900` (15 min hard timeout) |
| `ttlSecondsAfterFinished` | `86400` (1 day cleanup) |
| Name | `mendabot-agent-<first-12-chars-of-fingerprint>` |

### GitHub authentication

The agent uses a GitHub App (not a PAT). The init container calls
`get-github-app-token.sh` to exchange the App private key for a short-lived installation
token (valid 1 hour), written to a shared `emptyDir` volume, then consumed by the main
container for git clone and `gh` operations.

---

## Technology Stack

| Component | Technology | Reason |
|---|---|---|
| Controller language | Go 1.23 | Type-safe, idiomatic for Kubernetes ecosystem |
| Controller framework | controller-runtime v0.19.3 | Standard Kubernetes controller pattern |
| Logging | go.uber.org/zap | Structured logging |
| Agent base image | debian:bookworm-slim | Stable, rich apt ecosystem |
| kubectl | Official release binary | Standard cluster interaction |
| helm | Official release binary | GitOps repo uses Helm releases |
| flux | Official release binary | GitOps repo uses Flux |
| gh | GitHub CLI | PR creation, search, and commenting |
| opencode | Pinned GitHub release binary (not install script) | AI agent driver |
| Manifests | Kustomize | Matches talos-ops-prod GitOps pattern |
| Image registry | ghcr.io | Free, integrated with GitHub Actions |

---

## Worklog Requirements

Worklogs are **mandatory**. They are the institutional memory of this project. Every
meaningful session must produce a worklog entry. This is not optional.

### When to write a worklog

Write a worklog entry after **any** of the following:

- Completing a user story or part of one
- Making an architectural decision
- Discovering a bug or unexpected behaviour
- Completing a design document
- Running into a blocker
- Starting or finishing a feature branch
- Any session longer than 30 minutes of work

If in doubt: **write the worklog**.

### Worklog file naming

```
NNNN_YYYY-MM-DD_short-description.md
```

- `NNNN` is a zero-padded sequential number starting at `0001`
- Date is the actual date the work was done
- Description is lowercase, hyphen-separated, 3–6 words
- Next entry: check the highest existing number and increment by 1

Examples:
```
0001_2026-02-19_initial-design-and-docs.md
0002_2026-02-20_hld-review-and-revision.md
0003_2026-02-21_controller-tdd-foundation.md
```

### Worklog format

Every worklog entry must follow this exact structure:

```markdown
# Worklog: <Short Title>

**Date:** YYYY-MM-DD
**Session:** <brief description of what this session was about>
**Status:** Complete | In Progress | Blocked

---

## Objective

What was the goal of this session?

---

## Work Completed

### 1. <Area of work>
- Specific thing done
- Specific thing done

### 2. <Area of work>
- Specific thing done

---

## Key Decisions

List any decisions made and the rationale behind them. If a decision was
made without enough information, note that and flag it for follow-up.

---

## Blockers

List anything that is blocking progress. Include what information or action
is needed to unblock. If none, write "None."

---

## Tests Run

List test commands run and their outcomes. If no tests were run, explain why.

---

## Next Steps

What should the next session start with? Be specific enough that a fresh
context can pick up immediately without re-reading everything.

---

## Files Modified

List every file created or modified in this session.
```

### Worklog discipline rules

1. **Write it before ending the session** — not the next day. Memory degrades fast.
2. **Be specific** — vague entries like "worked on controller" are useless. Name the
   functions, the decisions, the line numbers if relevant.
3. **Document decisions with rationale** — not just what was decided, but why. Future
   sessions will need to understand the reasoning, not just the outcome.
4. **Record blockers immediately** — if you are blocked, write it down. Do not silently
   skip the entry.
5. **List every file touched** — this makes it trivial to audit what changed in a session.
6. **Next steps must be actionable** — "continue implementation" is not actionable.
   "Implement `fingerprintFor()` in `internal/controller/result_controller.go` and write
   tests first per TDD" is actionable.
7. **Never retroactively rewrite a worklog** — worklogs are append-only history. If
   something was wrong, note the correction in the next entry.

### Worklog index

`docs/WORKLOGS/README.md` must be kept up to date with a table of all entries:

```markdown
| # | Date | Description | Status |
|---|------|-------------|--------|
| 0001 | 2026-02-19 | Initial design and docs | Complete |
```

Update this table every time a new worklog is added.

---

## Development Workflow

### Before starting work

1. Read `README-LLM.md` (this file)
2. Read `docs/WORKLOGS/README.md` — scan the last 3–5 entries to understand current state
3. Read the latest worklog entry to find the documented next steps
4. Read the relevant LLD for the component you are about to touch
5. Check `docs/BACKLOG/` for the current story status

### During work

1. Write tests first — TDD, always
2. Use strongly-typed structs
3. Update backlog story checklists as you complete tasks
4. Commit at each logical unit of work with a descriptive message

### After completing work

1. Run all tests: `go test -timeout 30s -race ./...`
2. Verify tests pass
3. Update backlog story status
4. **Write a worklog entry** (see [Worklog Requirements](#worklog-requirements))
5. Update `docs/WORKLOGS/README.md` index table
6. Commit everything

---

## Multi-Agent Workflow

This section defines two agent roles and their workflows for collaborative or multi-step development.

**IMPORTANT:** These workflows are MANDATORY when working on epics, user stories, or complex multi-step tasks.

---

### Agent Role 1: Orchestrator Agent

**Purpose:** Coordinate multiple delegations to complete epics, stories, or complex multi-step tasks.

**When to use:**
- Working on epic-level features
- User story implementation requiring multiple sub-tasks
- Complex refactoring or architectural changes
- Coordinating work across multiple code areas

#### Orchestrator responsibilities

1. **Context distribution** — Ensure all delegations have access to critical documentation
2. **Scope definition** — Define clear boundaries, ownership, and integration points
3. **Quality enforcement** — Validate work meets standards through code review and testing
4. **Gap detection** — Identify and resolve integration gaps between sub-tasks
5. **Integration validation** — Ensure all components work together end-to-end
6. **Testing coordination** — Run comprehensive builds and tests across the entire repository
7. **Worklog management** — Create completion worklogs documenting the entire epic/story

#### Orchestrator workflow (11-step process)

Follow this workflow for all epic/story implementation tasks:

```
1. Context Setup
   └─> Delegate: "Read README-LLM.md, HLD, relevant LLDs, backlog story"
   └─> Include: Design constraints, architectural patterns, integration points
   └─> Define: Clear scope, ownership boundaries, expected deliverables

2. Implementation Delegation
   └─> Delegate: User story implementation with TDD requirements
   └─> Prompt detail level: "Fresh developer seeing codebase for first time"
   └─> Include: Specific file references, pattern examples, testing requirements

3. Code Review Delegation
   └─> Delegate: Skeptical code reviewer to validate implementation
   └─> Focus: Integration points, test coverage, gap detection, code quality
   └─> Requirement: Only code + tests count as proof of work (NOT status updates)
   └─> Output: Detailed gap report with code references and fix recommendations

4. Gap Remediation
   └─> Delegate: Fix ALL gaps identified in review (no matter how minor)
   └─> Include: Specific gap descriptions, code locations, fix strategies
   └─> Validate: Each fix with targeted tests

5. Iterative Validation
   └─> Repeat Steps 2–4 until ZERO gaps remain
   └─> Acceptance Criteria: "Story complete in spirit AND letter"
   └─> No compromises: All integration points validated, all tests passing

6. Build and Test Validation
   └─> Run ALL builds and tests, fix ANY failures
   └─> Commands:
       - go build ./...      # ALL packages must build
       - go test -timeout 30s -race ./...   # ALL tests must pass
   └─> NO TECH DEBT: Fix all failures regardless of relevance to current work
   └─> Zero tolerance: No pre-existing failures acceptable

7. Commit and Push
   └─> git add .
   └─> git commit -m "Descriptive message referencing story/epic"
   └─> git push origin HEAD

8. Worklog Creation
   └─> Create worklog in docs/WORKLOGS/ (see Worklog Requirements section)
   └─> Content: Summary, implementation details, test results, next steps
   └─> Commit worklog with code changes

9. Move to Next Story
   └─> Validate no implementation gaps between previous and current story
   └─> Common pitfall: Previous story built/tested but never wired into main code
   └─> If story file missing: Write it first before implementing
   └─> Repeat workflow from Step 1

10. Integration Gap Check
    └─> CRITICAL: Validate integration between stories
    └─> Ask: "Was previous story's code actually integrated into main codebase?"
    └─> Check: Import statements, registration calls, initialization code
    └─> Test: End-to-end flow through new and existing code paths

11. Final Validation
    └─> Run full repository test suite one final time
    └─> Confirm all backlog story checklists updated
    └─> Confirm worklog index updated
```

#### Orchestrator delegation guidelines

**Prompt quality standards:**
- Detail level: "Instructions for a developer seeing the codebase for the first time"
- Specificity: Include exact file paths, function names, pattern references
- Context: Provide architectural context, design decisions, trade-offs
- Boundaries: Clear scope limits, what is in/out of scope, integration points
- Examples: Reference similar implementations and established patterns

**Delegation prompt template:**

```
CONTEXT:
- Primary doc: README-LLM.md (your bible)
- Epic/Story: [Reference to docs/BACKLOG/epic-XX/]
- Design docs: [List all relevant HLD/LLD documents]
- Design constraints: [Architectural patterns, TDD, type safety, etc.]

SCOPE:
- Objective: [Clear, specific goal]
- Boundaries: [What is included, what is excluded]
- Integration points: [How this connects to existing code]
- Ownership: [Which files/packages this delegation owns]

REQUIREMENTS:
- MUST read README-LLM.md
- MUST read HLD.md and the relevant LLDs
- MUST follow TDD (tests first)
- MUST use established patterns
- MUST validate integration points
- MUST create worklog

DELIVERABLES:
1. [Specific deliverable 1 with acceptance criteria]
2. [Specific deliverable 2 with acceptance criteria]

SUCCESS CRITERIA:
- All tests passing (go test -timeout 30s -race ./...)
- All builds successful (go build ./...)
- Integration points validated
- Code follows established patterns
- Worklog created
```

#### Orchestrator principles

**Respect other agents:**
- Multiple agents may work simultaneously in the same repository
- NEVER perform indiscriminate destructive git operations (`git checkout .`, `git clean -fd`)
- Define clear ownership boundaries to avoid conflicts

**Thoroughness:**
- Proof of work = code + tests, NOT status updates
- Integration points MUST be identified and updated
- Sufficient end-to-end and integration tests for happy/unhappy paths
- NO gaps acceptable, no matter how minor

**Quality gates:**
- Code review before merge
- ALL tests passing before next story
- ALL builds successful before next story
- Worklog created before task closure

**Proper fixes only:**
- ALWAYS use the proper fix
- NEVER use workarounds, hacks, or shortcuts

---

### Agent Role 2: Delegation Agent

**Purpose:** Execute specific, well-scoped tasks as part of a larger epic or story.

**When to use:**
- Implementing a specific package or component
- Writing tests for a component
- Code review of another agent's work
- Fixing a specific bug or gap
- Integrating a component into the main codebase

#### Delegation agent responsibilities

1. **Context acquisition** — Read ALL assigned documentation (README-LLM.md, HLD, relevant LLDs, backlog story)
2. **Scope adherence** — Stay within defined boundaries; ask orchestrator if unclear
3. **Pattern following** — Use established patterns; check similar implementations
4. **TDD compliance** — Write tests FIRST, ensure they fail, then implement
5. **Integration awareness** — Identify and document integration points
6. **Quality standards** — Follow type safety, error handling, logging standards
7. **Worklog creation** — Document work performed if completing a task

#### Delegation agent workflow

**Standard implementation task:**

```
1. Read Required Documentation
   - README-LLM.md (MANDATORY — your bible)
   - Epic/story from docs/BACKLOG/
   - HLD.md and all relevant LLDs
   - Relevant design documents

2. Understand Context
   - Review delegation prompt carefully
   - Identify scope boundaries
   - Note integration points
   - Check similar implementations

3. Plan Implementation
   - Break down into sub-tasks
   - Identify test scenarios (happy + unhappy paths)
   - Note which patterns to follow
   - Identify dependencies

4. Write Tests FIRST (TDD)
   - Unit tests (happy paths)
   - Unit tests (unhappy paths)
   - Integration tests where applicable
   - Tests MUST fail initially

5. Implement
   - Follow established patterns
   - Use strongly-typed structs (never map[string]interface{})
   - Handle errors explicitly
   - Follow idiomatic Go

6. Validate
   - All tests pass
   - Code builds (go build ./...)
   - Integration points work
   - Follow-up questions documented

7. Create Worklog (if task complete)
   - Document what was done
   - Include test results
   - Note any issues or follow-up
   - See Worklog Requirements section

8. Report Back to Orchestrator
   - Clear completion status
   - Any gaps or uncertainties
   - Integration point validation status
   - Recommendations for next steps
```

**Code review task:**

```
1. Read Code with Skeptical Mindset
   - Assume nothing works until proven
   - Check every integration point
   - Verify test coverage (happy + unhappy)
   - Look for edge cases

2. Validate Against Standards
   - README-LLM.md rules followed?
   - TDD practised (tests first)?
   - Type safety maintained?
   - Patterns followed correctly?
   - Error handling comprehensive?

3. Integration Point Analysis
   - Are ALL integration points identified?
   - Are they properly tested?
   - Do end-to-end flows work?
   - Are there hidden dependencies?

4. Gap Identification
   - Document EVERY gap (no matter how minor)
   - Provide code references for each gap
   - Explain WHY it is a gap
   - Recommend HOW to fix it

5. Report Generation
   - Clear gap descriptions
   - Severity assessment
   - Fix recommendations with code examples
   - NO APPROVAL until all gaps fixed
```

#### Delegation agent principles

**Read first, ask later:**
- ALWAYS read README-LLM.md before ANY work
- ALWAYS read the epic/story README
- ALWAYS read ALL referenced HLD/LLD documents
- If information exists in docs, do not ask the orchestrator

**Follow patterns:**
- Check similar implementations in the codebase
- Use established patterns (controller-runtime reconcilers, strongly-typed CRD types, etc.)
- Do not invent new patterns without approval
- Consistency is critical

**Test-driven development:**
- Tests BEFORE code, always
- Tests must fail initially
- Happy AND unhappy paths
- Integration tests where applicable

**Quality standards:**
- Type safety (structs, not maps)
- Explicit error handling (never ignore errors)
- No TODOs or placeholders
- Complete implementations only

**Communication:**
- Report completion clearly
- Document gaps/uncertainties
- Ask questions when scope is unclear
- Provide recommendations for next steps

---

### Common failure modes

| Role | Failure Mode | Consequence |
|------|-------------|-------------|
| Orchestrator | Insufficient detail in delegation prompts | Delegation confusion, pattern violations |
| Orchestrator | Skipping integration validation | Code works in isolation but fails together |
| Delegation | Not reading README-LLM.md | Pattern violations, rule violations |
| Delegation | Scope creep | Conflicts with other agents, boundary violations |
| Both | No worklog | Lost context, incomplete task tracking |

---

## Common Commands

```bash
# Tidy dependencies
go mod tidy

# Build watcher binary
go build -o bin/mendabot-watcher ./cmd/watcher/

# Run all tests with timeout and race detector
go test -timeout 30s -race ./...

# Run tests with coverage
go test -timeout 30s -cover ./...

# Format code
go fmt ./...

# Static analysis
go vet ./...

# Build agent image locally
docker build -f docker/Dockerfile.agent -t mendabot-agent:dev .

# Apply Kustomize manifests (dry-run)
kubectl apply -k deploy/kustomize/ --dry-run=client

# Apply manifests
kubectl apply -k deploy/kustomize/
```

---

## Branch Management

**Active branches:**

| Branch | Purpose | Status | Created |
|--------|---------|--------|---------|
| `main` | Stable code | Active | 2026-02-19 |
| `feature/epic11-13-deferred` | Epic 11 (cascade prevention) + Epic 13 (multi-signal correlation) — deferred | Deferred | 2026-02-23 |
| `feature/epic12-security-remediation` | Epic 12 security gap remediation (findings 001–013) | Active | 2026-02-23 |
| `feature/epic16-annotation-control` | Epic 16 per-resource annotation control (enabled, skip-until, priority) | Active | 2026-02-24 |

**Merged branches:**

| Branch | Purpose | Merged | Commit |
|--------|---------|--------|--------|
| `feature/epic09-native-provider` | Native cluster provider (replaces k8sgpt) | 2026-02-22 | df59899 |
| `feature/epic10-helm-chart` | Helm chart packaging (epic10) | 2026-02-23 | 2dec0ae |
| `feature/epic11-fixes` | Epic 11 complete: EventRecorder (3 events), 10-gap review, Grafana dashboard, alert rules | 2026-02-23 | 9a8477a |

**Branch naming:**
- Feature: `feature/short-description`
- Bugfix: `bugfix/issue-description`
- Hotfix: `hotfix/critical-issue`

**Branch workflow:**
1. Create branch from `main`
2. Add to the active branches table above
3. Work in branch with regular commits
4. Write a worklog entry before merging
5. Merge to `main` when complete and all tests pass
6. Move to merged table, delete branch

---

## Testing Requirements

### TDD workflow

```
1. Write test first
2. Run — must fail
3. Write minimal code to pass
4. Run — must pass
5. Refactor
```

### Coverage requirements

- Multiple happy path cases
- Multiple unhappy path cases
- Edge cases (empty fields, nil slices, very long strings)
- Error conditions

### Table-driven tests

Use table-driven tests with `t.Run()` for any function with multiple input cases:

```go
func TestFingerprintFor(t *testing.T) {
    tests := []struct {
        name string
        spec ResultSpec
        want string
    }{
        {"empty errors", ResultSpec{...}, "abc123..."},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := fingerprintFor(tt.spec)
            if got != tt.want {
                t.Errorf("got %s, want %s", got, tt.want)
            }
        })
    }
}
```

### Always use timeout

```bash
# Good
go test -timeout 30s ./...

# Bad — can hang forever
go test ./...
```

### envtest integration tests

Integration tests in `internal/controller/` share a single envtest process. Two rules
apply to all tests in this package:

**Rule 1 — CRD testdata maintenance.** `testdata/crds/remediationjob_crd.yaml` is a
manually maintained copy of the CRD schema loaded by envtest. The Kubernetes API server
enforces this schema and silently strips unknown fields during status and object patches.
The fake client used in unit tests does NOT enforce schema, which means a missing field
will pass unit tests but fail integration tests.

When adding a field to `RemediationJobStatus` or `RemediationJobSpec` in
`api/v1alpha1/remediationjob_types.go`, you MUST also add the corresponding entry to
`testdata/crds/remediationjob_crd.yaml`:

- New `status` fields go under `spec.versions[0].schema.openAPIV3Schema.properties.status.properties`
- New `spec` fields go under `spec.versions[0].schema.openAPIV3Schema.properties.spec.properties`

Use the correct OpenAPI type: `{type: string}`, `{type: boolean}`, `{type: integer}`,
`{type: string, format: date-time}`.

Example: when `isSelfRemediation bool` was added to `RemediationJobSpec`, the
corresponding entry added to the CRD was:

```yaml
              isSelfRemediation: {type: boolean}
```

**Rule 2 — Pre-test cleanup for deterministic object names.** When a test creates a
Kubernetes object with a name derived from a fixed constant (e.g. a `batch/v1` Job
named `mendabot-agent-<fingerprint[:12]>`), add a pre-test delete at the start of the
test body, before creating any objects:

```go
// Pre-test cleanup: delete any stale object from a previous run.
_ = c.Delete(ctx, &batchv1.Job{ObjectMeta: metav1.ObjectMeta{
    Name:      "mendabot-agent-" + fp[:12],
    Namespace: integrationCtrlNamespace,
}})
```

Ignore the error (`_ =`): a not-found result is the normal case and must not fail the
test. Do not rely solely on `t.Cleanup` for this — `t.Cleanup` runs *after* the test
and cannot protect the next run if cleanup failed or the process was interrupted.

**Rule 3 — Job namespace must follow the RemediationJob.** Helper functions that build
`batch/v1` Job objects for tests (e.g. `newIntegrationJob`) must set
`Namespace: rjob.Namespace`, not a hardcoded string like `"default"`. Hardcoding the
namespace causes jobs to land in the wrong namespace when tests run rjobs in a dedicated
namespace, which pollutes label-based list queries (e.g. max-concurrent counts) used by
other tests sharing the same envtest process.

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.1 | 2026-02-20 | Added Multi-Agent Workflow section (Orchestrator + Delegation Agent) |
| 1.0 | 2026-02-19 | Initial creation |
