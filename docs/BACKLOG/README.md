# docs/BACKLOG/

## Purpose

Implementation backlog organised by epic. Each epic folder contains a README describing
the epic and individual story files for each unit of work.

## Rules

- Read the epic README before starting any story in that epic
- Update story checklist items `[ ]` → `[x]` as you complete tasks
- Mark story status as `In Progress` when you start it, `Complete` when done
- Stories within an epic should generally be worked in the order listed in the epic README
- Do not start a new epic until all blocking epics are complete (see dependency table below)

## Epic Overview

| Epic | Folder | Description | Depends On | Status |
|------|--------|-------------|------------|--------|
| epic00 — Foundation | [epic00-foundation/](epic00-foundation/) | Go module, project structure, config, CI skeleton | — | Not Started |
| epic00.1 — Interfaces | [epic00.1-interfaces/](epic00.1-interfaces/) | RemediationJob CRD types, JobBuilder interface, reconciler skeletons, envtest suite, fakes | epic00 | Not Started |
| epic01 — Controller | [epic01-controller/](epic01-controller/) | SourceProviderReconciler + RemediationJobReconciler | epic00, epic00.1 | Not Started |
| epic02 — Job Builder | [epic02-jobbuilder/](epic02-jobbuilder/) | Agent Job spec construction from RemediationJob | epic00.1, epic01 | Not Started |
| epic03 — Agent Image | [epic03-agent-image/](epic03-agent-image/) | Dockerfile, tool install, entrypoint script | epic00 | Not Started |
| epic04 — Deploy | [epic04-deploy/](epic04-deploy/) | Kustomize manifests, RBAC, Secrets | epic01, epic02, epic03 | Not Started |
| epic05 — Prompt | [epic05-prompt/](epic05-prompt/) | OpenCode prompt design and ConfigMap | epic04 | Not Started |
| epic06 — CI/CD | [epic06-ci-cd/](epic06-ci-cd/) | GitHub Actions workflows for both images | epic03, epic00 | Not Started |
| epic07 — Technical Debt | [epic07-technical-debt/](epic07-technical-debt/) | Issues and improvements discovered during implementation | — | Not Started |

## Implementation Order

```
epic00-foundation
    ├── epic00.1-interfaces
    │       ├── epic01-controller
    │       │         └── epic02-jobbuilder
    │       │                     └── epic04-deploy ──┐
    │       └── (fakes used by epic01 unit tests)     │
    ├── epic03-agent-image ──────────────────────────┤
    │                                                  └── epic05-prompt
    └── epic06-ci-cd (parallel with epic01+)
```

## Story Status Key

- `Not Started` — work has not begun
- `In Progress` — actively being worked on
- `Complete` — all acceptance criteria met, tests passing
- `Blocked` — cannot proceed; see story for blocker details
