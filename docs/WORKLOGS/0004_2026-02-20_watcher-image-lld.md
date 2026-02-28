# Worklog 0004 — 2026-02-20 — Watcher Image LLD and Status Update

**Session date:** 2026-02-20
**Author:** LLM / Human
**Duration:** Short session

---

## What Was Done

### 1. Resumed from session summary

Resumed from the end-of-session summary left by worklog 0003. The outstanding gap identified
in that session was **F-041: Watcher Dockerfile entirely missing** — a note had been added
to HLD §4.1 acknowledging the gap but no dedicated LLD document existed.

---

### 2. Updated STATUS.md

Updated `docs/STATUS.md` to reflect:
- Two design reviews complete (worklogs 0002 and 0003)
- WATCHER_IMAGE_LLD.md now exists (gap closed)
- "Remaining Design Work" section updated from a cross-reference review item to a statement
  that design is complete

---

### 3. Wrote WATCHER_IMAGE_LLD.md

Created `docs/DESIGN/lld/WATCHER_IMAGE_LLD.md` (new file). Contents:

| Section | What it covers |
|---|---|
| §1 Overview | Purpose; design principles (multi-stage, debian-slim, non-root, no extra tools, CGO disabled, read-only rootfs) |
| §2 Build Arguments | `GO_VERSION`, `TARGETARCH`, `WATCHER_VERSION` |
| §3 Dockerfile | Full `docker/Dockerfile.watcher` — builder stage (`golang:1.23-bookworm`) + runtime stage (`debian:bookworm-slim`); non-root `watcher` uid=1000; single binary `/usr/local/bin/watcher` |
| §4 Why debian-slim not distroless | Consistency with agent image; distroless option noted for operators who need it |
| §5 cmd/watcher entry point | Env-only config; ports 8080 (metrics) and 8081 (healthz/readyz) — matches DEPLOY_LLD §8 |
| §6 Image tagging strategy | `latest` / `sha-<7char>` / `v<semver>` — matches agent image policy |
| §7 Multi-architecture | `TARGETARCH` ARG + `GOOS=linux GOARCH=${TARGETARCH}` — no extra toolchain needed |
| §8 Build verification | Three smoke tests: binary present, binary starts and logs "starting manager", uid=1000 |
| §9 Security context | Documents that `readOnlyRootFilesystem: true` is already in DEPLOY_LLD; notes the `/tmp` emptyDir caveat for single-replica no-leader-election deployments |
| §10 Version embedding | `ldflags -X main.Version=${WATCHER_VERSION}`; startup log pattern |

---

### 4. Consistency check

Verified WATCHER_IMAGE_LLD against HLD §4.1 and DEPLOY_LLD §8:

| Check | Result |
|---|---|
| Image name `ghcr.io/lenaxia/mechanic-watcher:latest` | ✅ Match |
| Ports 8080 (metrics) and 8081 (healthz/readyz) | ✅ Match |
| securityContext fields (runAsUser 1000, readOnlyRootFilesystem, allowPrivilegeEscalation false, capabilities ALL drop) | ✅ Match |
| Configuration via env vars (no flags) | ✅ Consistent with DEPLOY_LLD §8 env block |
| Single binary, no extra tools | ✅ Consistent with HLD §4.1 description |

No contradictions found.

---

## Files Modified

| File | Change |
|---|---|
| `docs/STATUS.md` | Updated last-updated date, design doc table, remaining work section |
| `docs/DESIGN/lld/WATCHER_IMAGE_LLD.md` | **New file** — resolves F-041 |

---

## Design State After This Session

| Document | Status |
|---|---|
| `HLD.md` | Complete |
| `lld/CONTROLLER_LLD.md` | Complete |
| `lld/JOBBUILDER_LLD.md` | Complete |
| `lld/AGENT_IMAGE_LLD.md` | Complete |
| `lld/DEPLOY_LLD.md` | Complete |
| `lld/PROMPT_LLD.md` | Complete |
| `lld/WATCHER_IMAGE_LLD.md` | Complete — new this session |

All six LLDs and the HLD are now complete. Two design reviews have been completed; all 43
confirmed findings resolved. Design is ready for implementation.

---

## Next Steps

Design is complete. Implementation can begin.

**Start at:** `docs/BACKLOG/epic00-foundation/stories/STORY_01_module_setup.md`

**Implementation sequence:**
```
epic00 (foundation) → epic01 (controller) → epic02 (job builder) → epic03 (agent image)
→ epic04 (deploy — write stories first) → epic05 (prompt — write stories first)
→ epic06 (ci/cd — write stories first)
```

Each story: write test → fail → implement → pass → refactor (TDD).

---

## Persistent Constraints (carry forward)

- No alpine — debian-slim or distroless only
- `GitInitImage` config field does not exist — init container uses `AgentImage`
- Token path: `/workspace/github-token`
- `FINDING_NAMESPACE` must be in fingerprint hash and all env var tables
- `envsubst` must use explicit var list — never bare `envsubst`
- GitHub App auth (not PAT) — JWT → token exchange
- JWT `iss` field must be a JSON number (integer), not a string
- GitHub App private key: init container only — never in main container env
- All binary downloads: checksum verification in Dockerfile
- Agent Job pods: securityContext (non-root, read-only rootfs)
- `opencode run --file <path>` — never `opencode run "$(cat ...)"`
- Images to `ghcr.io/lenaxia/`
- Watcher controller: return nil, no requeue after successful reconcile
- Deleted Results must evict fingerprint from processed map
- Watcher Role must have `delete` verb for Jobs
- Stories for epic04–06 written at start of each epic, not upfront
- Worklog entry required for every session
- No implementation until design is verified complete (now satisfied)
