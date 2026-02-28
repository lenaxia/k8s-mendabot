# Worklog: Design Review and Fixes

**Date:** 2026-02-19
**Session:** Skeptical design review of all LLDs + HLD, followed by remediation of all findings
**Status:** Complete

---

## Objective

A thorough adversarial review of the HLD and all five LLDs was delegated to a skeptical
architect agent. The agent produced a report of 37 findings (7 Critical, 21 Major, 9 Minor).
This session resolved all findings by updating the affected design documents. No
implementation code was written.

---

## Work Completed

### 1. Design review (delegated agent)

A full review of HLD.md, all 5 LLDs, README-LLM.md, and all 30 backlog stories was
performed. The full report is preserved at:
`~/.local/share/opencode/tool-output/tool_c79f0f14b001IX0Zel82d4HXC6`

Summary of findings before fixes:
- 7 Critical blockers
- 21 Major issues
- 9 Minor issues

### 2. Critical fixes

| Finding | Fix |
|---|---|
| Init container used `alpine/git` (missing openssl/curl, no alpine policy) | Init container now uses the same `AgentImage` (debian-slim). No separate init image needed. `GitInitImage` config field removed. |
| GitHub token path inconsistent: `/tmp/github-token`, `/tmp/shared/github-token`, `/workspace/github-token` across documents | Canonicalised to `/workspace/github-token` everywhere. |
| `FINDING_NAMESPACE` missing from all data models | Added `FINDING_NAMESPACE` (from `result.Namespace`) to HLD config table, JOBBUILDER_LLD env injection, PROMPT_LLD variable table, prompt template, and fingerprint algorithm. |
| Fingerprint did not include namespace — cross-namespace collisions possible | Fingerprint now: `sha256(namespace + kind + parentObject + sorted(error[].text))`. Updated CONTROLLER_LLD algorithm and test list. |
| OpenCode CLI interface and LLM config entirely unspecified | AGENT_IMAGE_LLD §5 now documents `opencode run --file <path>` invocation and that OpenCode reads `OPENAI_API_KEY`, `OPENAI_BASE_URL`, `OPENAI_MODEL` from environment. |
| Git identity never configured — every `git commit` would fail | Added `GIT_AUTHOR_NAME`, `GIT_AUTHOR_EMAIL`, `GIT_COMMITTER_NAME`, `GIT_COMMITTER_EMAIL` as `ENV` in the Dockerfile. |
| Three incompatible entrypoint models across three documents; `envsubst` unsafe for arbitrary `$` content | Single canonical model: `ENTRYPOINT ["/usr/local/bin/agent-entrypoint.sh"]`. Script uses `envsubst "$VARS"` with explicit variable list to prevent corruption of `FINDING_ERRORS`/`FINDING_DETAILS`. Prompt passed via `opencode run --file`. |

### 3. Major fixes

| Finding | Fix |
|---|---|
| Hard Rule 3 ("exactly one PR") conflicted with Decision Tree ("exit after comment, no PR") | Hard Rule 3 reworded to two mutually exclusive outcomes: (a) existing PR found → comment + exit; (b) no existing PR → open exactly one. Decision Tree updated to reference Rule 3a/3b. |
| `FINDING_NAME` inconsistently shown with and without `namespace/` prefix | Defined as plain name (no prefix). `FINDING_NAMESPACE` is the separate variable. All docs updated. |
| Two incompatible `get-github-app-token.sh` scripts | Single script: bash + jq, outputs to stdout. Init container calls it and writes output to `/workspace/github-token`. Main container reads the file. AGENT_IMAGE_LLD §4 is now the sole definition. |
| `MAX_CONCURRENT_JOBS` loaded into Config but enforcement mechanism never described | CONTROLLER_LLD §5 now specifies: list Jobs with `managed-by: mechanic-watcher` label, count active/pending, requeue 30s if at limit. |
| Secret key names (`api-key`, `app-id`) differed from env var names (`OPENAI_API_KEY`, `GITHUB_APP_ID`) with no mapping documented | JOBBUILDER_LLD §4 now explicitly documents the mapping and notes that `secretKeyRef.key` must use Secret key names. |
| `GITOPS_MANIFEST_ROOT` missing — manifest search path hardcoded to `/repo/kubernetes/` | Added `GITOPS_MANIFEST_ROOT` as a required watcher env var. Added to HLD config table, CONTROLLER_LLD Config struct, JOBBUILDER_LLD Config struct + env injection, DEPLOY_LLD Deployment manifest, STORY_02_config.md, PROMPT_LLD variable table, and prompt template. |
| PR dedup used unreliable `--search` text match; TTL vulnerability undocumented | Step 1 now uses `gh pr list --json --jq` with exact `headRefName` match. TTL window documented in PROMPT_LLD §3. |
| `needs-human-review` label not guaranteed to exist | DEPLOY_LLD §6.3 added with `gh label create` one-time setup command. |
| `sops`/`age` tools installed but no key material injected — silently non-functional | AGENT_IMAGE_LLD tool table and §3 note clarify they require separately mounted key material. PROMPT_LLD environment section notes agent should skip encrypted files if key absent. |
| `opencode` installed via unpinned `curl | bash` install script | Replaced with pinned release binary fetch using `ARG OPENCODE_VERSION`. |
| `AGENT_NAMESPACE` configurable but Role is namespace-scoped — setting it to a different namespace silently breaks Job creation | Documented as a constraint in HLD §12 and DEPLOY_LLD §8 comment. |

### 4. Minor fixes

| Finding | Fix |
|---|---|
| `describe` in ClusterRole verbs | Removed from DEPLOY_LLD §5.1 and HLD §7. |
| README-LLM.md backlog path diagram used wrong directory names | Updated to `epic00-foundation/` etc. |
| `AutoRemediationStatus` in vendored types but never used | Removed from `ResultSpec` in CONTROLLER_LLD §3. Noted in STORY_04 context via Config struct update. |
| Requeue-after-5min for already-processed Results — unnecessary steady-state load | Changed to `return nil` (no requeue). Documented in CONTROLLER_LLD §5. |
| `talosctl` installed but no credentials injected | AGENT_IMAGE_LLD §3 note added explaining `TALOSCONFIG` must be mounted separately. |
| `json.Marshal` error discarded with `_` and falsely called "never errors" | Error now handled explicitly (panic with message). CONTROLLER_LLD §9 updated. |
| `stern` installed but never referenced in prompt | AGENT_IMAGE_LLD tool table notes it is available; prompt environment section lists all tools including `stern`. The prompt does not prescribe its use but the agent may use it as needed. |

---

## Key Decisions

| Decision | Rationale |
|---|---|
| Init container uses the same `AgentImage`, not a separate image | Eliminates the alpine dependency, removes a config field, ensures `bash`/`jq`/`openssl` are always available |
| `envsubst "$VARS"` with explicit list, not bare `envsubst` | Bare `envsubst` replaces any `$word` in the input, which corrupts Helm template variables and shell variable references in `FINDING_ERRORS`/`FINDING_DETAILS` |
| `opencode run --file` not `opencode run "$(cat ...)"` | Shell argument quoting + word-splitting is unsafe for arbitrary LLM-generated content |
| Fingerprint includes namespace | Two Deployments named `my-app` in different namespaces would otherwise collide |
| `GITOPS_MANIFEST_ROOT` as a required env var | Makes the repo layout configurable, required for the upstream contribution goal |
| `needs-human-review` label setup is a manual one-time step | Automating it would require the watcher to have GitHub API write access, which is out of scope |
| talosctl included but credentials not injected by default | User explicitly requested talosctl be kept. It is functional when `TALOSCONFIG` is mounted; documented clearly |

---

## Blockers

None — all 37 findings resolved.

---

## Tests Run

No tests run — this session was documentation-only.

---

## Next Steps

The design is now ready for implementation. Begin with **epic00-foundation**:

1. Start with `STORY_01_module_setup.md` — add `controller-runtime`, `go.uber.org/zap`,
   and k8sgpt-operator API types to `go.mod`, then `go mod tidy`
2. Follow with `STORY_02_config.md` — implement `internal/config/config.go` with
   `FromEnv()`. Fields: `GitOpsRepo`, `GitOpsManifestRoot`, `AgentImage`, `AgentNamespace`,
   `AgentSA`, `LogLevel`, `MaxConcurrentJobs`. TDD: write tests first.
3. Continue through epic00 stories in order before touching epic01

**Reminder:** `AGENT_NAMESPACE` must equal the watcher's own namespace (default:
`mechanic-watcher`). Enforce this with a validation check in `FromEnv()` or document
clearly that setting it to a different value will break Job creation.

---

## Files Modified

| File | Action |
|---|---|
| `docs/DESIGN/HLD.md` | Updated — init image, token path, FINDING_NAMESPACE, fingerprint algo, AGENT_NAMESPACE constraint, config table (added GITOPS_MANIFEST_ROOT, FINDING_NAMESPACE, secret key names), RBAC (removed describe), data flow example |
| `docs/DESIGN/lld/CONTROLLER_LLD.md` | Updated — fingerprint (namespace added), Config struct (added GitOpsManifestRoot), reconcile loop (removed requeue, added MAX_CONCURRENT_JOBS enforcement), error table, removed AutoRemediationStatus from ResultSpec, test list |
| `docs/DESIGN/lld/JOBBUILDER_LLD.md` | Updated — Config struct (removed GitInitImage, added GitOpsManifestRoot), init container (image → AgentImage, name → git-token-clone), main container (added command, FINDING_NAMESPACE, GITOPS_MANIFEST_ROOT, secret key mapping note), init script (replaced sh+grep with bash+jq), FINDING_ERRORS serialisation (Sensitive redaction), test list |
| `docs/DESIGN/lld/AGENT_IMAGE_LLD.md` | Updated — tool table (envsubst, talosctl/sops/age notes), Dockerfile (added OPENCODE_VERSION ARG, pinned binary fetch, envsubst/gettext-base, GIT_* ENV, agent-entrypoint.sh COPY, ENTRYPOINT), §4 get-github-app-token.sh (clarified stdout output + single version), §5 entrypoint (replaced with canonical agent-entrypoint.sh design + OpenCode LLM config docs), §8 smoke test (added envsubst, entrypoint scripts) |
| `docs/DESIGN/lld/PROMPT_LLD.md` | Updated — entrypoint reference, prompt template (added FINDING_NAMESPACE, GITOPS_MANIFEST_ROOT, gh auth moved to entrypoint, Step 1 uses exact branch match, Steps 2–6 use ${FINDING_NAMESPACE} var, Step 5 uses ${GITOPS_MANIFEST_ROOT}), Hard Rules (Rule 3 reworded to 3a/3b), Decision Tree (references 3a/3b), §3 variable table (added FINDING_NAMESPACE, GITOPS_MANIFEST_ROOT, fixed FINDING_NAME), envsubst call (restricted vars), PR dedup TTL note |
| `docs/DESIGN/lld/DEPLOY_LLD.md` | Updated — ClusterRole (removed describe verb), Deployment manifest (added GITOPS_MANIFEST_ROOT, AGENT_NAMESPACE comment), §6.3 (added needs-human-review label setup) |
| `docs/BACKLOG/epic00-foundation/STORY_02_config.md` | Updated — Config struct (removed GitInitImage, added GitOpsManifestRoot, updated AGENT_NAMESPACE note) |
| `README-LLM.md` | Updated — backlog directory paths (added epicNN- prefix), fingerprint formula (added namespace) |
| `docs/WORKLOGS/0002_2026-02-19_design-review-fixes.md` | Created |
