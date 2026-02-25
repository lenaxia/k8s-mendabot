# Governance

## Project Status

k8s-mendabot is an open-source project currently maintained by
[lenaxia](https://github.com/lenaxia). The project is actively seeking
co-maintainers from other organizations. If you use k8s-mendabot in production
and are interested in co-maintaining, open a GitHub issue labelled `governance`
to start the conversation.

The authoritative list of current maintainers and committers is in
[MAINTAINERS.md](MAINTAINERS.md).

---

## Roles

### Maintainer

Maintainers are responsible for:

- Setting product direction and prioritising the backlog
- Reviewing and merging pull requests
- Cutting releases and publishing container images
- Running security reviews and signing off on accepted risks
- Enforcing the [Code of Conduct](CODE_OF_CONDUCT.md)
- Maintaining the CI/CD pipeline and release infrastructure

**Current maintainers:** see [MAINTAINERS.md](MAINTAINERS.md)

### Committer

Committers are trusted contributors with review and merge rights. They are
expected to:

- Review pull requests in a timely manner
- Participate in design discussions
- Help triage issues and support new contributors

Committers are listed in [MAINTAINERS.md](MAINTAINERS.md).

### Contributor

Anyone who submits a pull request, opens an issue, or participates in
discussion is a contributor. Contributors do not have merge rights.

---

## Contributor Ladder

The project uses a three-tier ladder: Contributor → Committer → Maintainer.

### Contributor → Committer

A contributor may be nominated for committer status after demonstrating
sustained, high-quality engagement with the project. Criteria:

- At least 5 pull requests merged over a period of at least 60 days
- Participation in at least one architectural or design discussion
- No unresolved Code of Conduct concerns
- Familiarity with the project's testing, security, and governance practices

**Process:** Any maintainer may nominate a contributor by opening a GitHub
issue labelled `governance` with a summary of the nominee's contributions.
Existing maintainers discuss and approve by consensus. The new committer is
added to [MAINTAINERS.md](MAINTAINERS.md) via a pull request.

### Committer → Maintainer

A committer may be nominated for maintainer status after demonstrating
ownership across multiple areas of the project. Additional criteria beyond
committer requirements:

- Track record of reviewing and merging pull requests
- Demonstrated ability to make product and architectural decisions
- Willingness to take on release and security responsibilities
- Affiliation with an organization different from existing maintainers is
  preferred but not required

**Process:** Same as committer nomination — open a `governance` issue, discuss
with existing maintainers, approve by consensus.

### Inactive Maintainers / Emeritus

A maintainer who has not participated in the project (code reviews, issues,
releases, or community discussions) for 6 months may be moved to Emeritus
status. Emeritus maintainers are listed in [MAINTAINERS.md](MAINTAINERS.md)
and are always welcome to return to active status.

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
