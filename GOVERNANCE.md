# Governance

## Project Status

k8s-mendabot is a personal / small-team open-source project maintained by
[lenaxia](https://github.com/lenaxia). It does not (yet) have a formal
governance body. This document describes how decisions are made, how
maintainership works, and what contributors can expect.

---

## Roles

### Maintainer

The project maintainer is responsible for:

- Setting product direction and prioritising the backlog
- Reviewing and merging pull requests
- Cutting releases and publishing container images
- Running security reviews and signing off on accepted risks
- Enforcing the [Code of Conduct](CODE_OF_CONDUCT.md)
- Maintaining the CI/CD pipeline and release infrastructure

**Current maintainer:** [lenaxia](https://github.com/lenaxia)

### Contributor

Anyone who submits a pull request, opens an issue, or participates in
discussion is a contributor. Contributors do not have merge rights.

There is currently no committer tier between contributor and maintainer.
If the project grows to warrant it, this document will be updated.

---

## Decision Making

### Day-to-day decisions

Routine decisions — bug fixes, documentation improvements, dependency upgrades,
minor feature work — are made by the maintainer. A pull request that follows
[CONTRIBUTING.md](CONTRIBUTING.md), passes CI, and receives maintainer review
approval is eligible to merge.

### Architectural decisions

Changes that affect the core data flow, RBAC model, CRD schema, agent
entrypoint, or security properties require:

1. A design document or updated LLD in `docs/DESIGN/lld/` **before**
   implementation begins.
2. Explicit maintainer sign-off in the pull request.
3. A worklog entry in `docs/WORKLOGS/` after the work is complete.

If you are a contributor proposing an architectural change, open an issue first
to discuss the design before investing implementation time.

### Security decisions

Any change to the security model — RBAC manifests, Dockerfiles, the agent
entrypoint, redaction logic, or the agent prompt — requires:

1. A re-read of [`docs/SECURITY/THREAT_MODEL.md`](docs/SECURITY/THREAT_MODEL.md).
2. A review following [`docs/SECURITY/PROCESS.md`](docs/SECURITY/PROCESS.md)
   (or a documented partial review scoped to the affected phases).
3. Any new findings triaged in a security report committed to
   `docs/SECURITY/`.

No HIGH or CRITICAL finding may be left Open without explicit written sign-off
from the maintainer in the report.

---

## Roadmap and Backlog

The product backlog is maintained in [`docs/BACKLOG/`](docs/BACKLOG/) and
the high-level roadmap is in `README.md`. Features are tracked as epics with
user stories, acceptance criteria, and value/complexity ratings before
implementation begins.

**Roadmap input:** Open a GitHub issue labelled `roadmap` to propose a feature
or epic. The maintainer will triage it into the backlog or decline with a
reason.

**Feature status:** The backlog and
[`docs/BACKLOG/FEATURE_TRACKER.md`](docs/BACKLOG/FEATURE_TRACKER.md) are the
authoritative source of feature status. `Planned`, `Evaluated`, and `Shipped`
are the three states on the public roadmap table in `README.md`.

---

## Releases

Releases follow [Semantic Versioning](https://semver.org):

| Change | Version bump |
|---|---|
| Bug fix or security patch | Patch (`0.3.x → 0.3.x+1`) |
| New feature, backward-compatible | Minor (`0.3.x → 0.4.0`) |
| Breaking change (CRD schema, API, required config) | Major (`0.x → 1.0`) |

**Pre-1.0:** The project is currently in the `v0.3.x` series. Minor version
bumps may include breaking changes while the API is still stabilising.
Breaking changes will always be called out explicitly in the release notes.

### Release process

1. All CI checks pass on `main`.
2. The Helm chart `version` and `appVersion` in `Chart.yaml` are updated.
3. Container images are built and pushed to `ghcr.io/lenaxia/` by CI on the
   release tag.
4. A GitHub release is created with a changelog entry (see
   [CHANGELOG.md](CHANGELOG.md)).
5. Both `mendabot-watcher` and `mendabot-agent` images are scanned with Trivy
   before the release is published. Fixable `CRITICAL`/`HIGH` CVEs block
   the release.

---

## Dependencies and Compatibility

- **Kubernetes:** >= 1.28 (tested against 1.28 – 1.35)
- **Helm:** >= 3.14
- **Go:** 1.24+ (for building from source)
- **controller-runtime:** pinned in `go.mod`; updated per standard module
  upgrade process with `govulncheck` and full test suite validation

Breaking changes to the `RemediationJob` CRD schema (e.g. adding required
fields, removing fields, changing field types) will be called out in the
release notes with a migration path.

---

## Institutional Memory

This project uses structured worklogs to maintain institutional memory across
AI-assisted and human development sessions.

- Every significant session produces a worklog in `docs/WORKLOGS/`.
- Design decisions are recorded in `docs/DESIGN/` with rationale.
- Security review outcomes are committed to `docs/SECURITY/`.
- Backlog story checklists are updated as work is completed.

This practice is mandatory for all contributors, human and AI alike. It is what
makes the project auditable and allows new contributors (or a fresh context) to
understand the current state without re-reading all history.

---

## Amendments

This document is maintained in the repository. Changes to governance are
proposed via pull request and require maintainer approval. Significant changes
will be noted in [CHANGELOG.md](CHANGELOG.md).
