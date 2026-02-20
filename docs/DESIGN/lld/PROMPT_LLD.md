# Domain: OpenCode Prompt — Low-Level Design

**Version:** 1.2
**Date:** 2026-02-19
**Status:** Implementation Ready
**HLD Reference:** [Section 10](../HLD.md)
**Sink Reference:** [SINK_PROVIDER_LLD.md](SINK_PROVIDER_LLD.md)

---

## 1. Overview

### 1.1 Purpose

Defines the exact prompt given to OpenCode when the agent Job runs. The prompt is the
contract between the watcher (which knows about the finding) and the agent (which knows
how to investigate and take action). Getting this right is as important as any other
component — a bad prompt produces useless PRs.

### 1.2 Design Principles

- **Explicit investigation steps** — the agent must not guess what to do; every step is
  named in order
- **Hard rules over suggestions** — constraints (no commits to main, no secrets, one PR)
  are stated as rules, not preferences
- **PR deduplication is mandatory, not optional** — the first step is always to check for
  existing PRs; this is the primary defence against duplicates on watcher restart
- **Graceful degradation** — if the agent cannot determine a safe fix, it opens an
  investigation-report PR rather than guessing
- **Environment variable interpolation** — the prompt template uses `${VAR}` placeholders
  that are substituted at Job startup via `envsubst`

---

## 2. Prompt Template

The following is the content of `deploy/kustomize/configmap-prompt.yaml`'s `prompt.txt`
key. It is mounted at `/prompt/prompt.txt` in the agent container.

The agent container runs `agent-entrypoint.sh` (baked into the image), which:

1. Authenticates `gh` using the token at `/workspace/github-token`
2. Runs `envsubst` with an explicit variable list to substitute known variables only
3. Calls `opencode run --file /tmp/rendered-prompt.txt`

See [AGENT_IMAGE_LLD.md §5](AGENT_IMAGE_LLD.md) for the full entrypoint script.

---

```
You are an SRE agent running inside a Kubernetes cluster. The k8sgpt-operator has
identified a problem and your job is to investigate it, understand the root cause, and
open a pull request on the GitOps repository with a proposed fix.

=== FINDING ===

Kind:         ${FINDING_KIND}
Resource:     ${FINDING_NAME}
Namespace:    ${FINDING_NAMESPACE}
Parent:       ${FINDING_PARENT}
Fingerprint:  ${FINDING_FINGERPRINT}

Errors detected:
${FINDING_ERRORS}

AI analysis from k8sgpt:
${FINDING_DETAILS}

=== ENVIRONMENT ===

- You are running inside the Kubernetes cluster with read-only access via in-cluster
  ServiceAccount. Use kubectl with no kubeconfig flags — it will work automatically.
- The GitOps repository has been cloned to /workspace/repo.
- The GitOps repository is: ${GITOPS_REPO}
- The GitOps manifests root within the repo is: ${GITOPS_MANIFEST_ROOT}
- Your GitHub token is already loaded — gh commands work immediately.
- All tools available: kubectl, k8sgpt, helm, flux, talosctl, kustomize, gh, git,
  jq, yq, kubeconform, stern, sops, age.
- talosctl requires a talosconfig to be mounted — if absent, skip Talos-specific steps.
- sops/age require decryption key material to be mounted — if absent, skip encrypted files.

=== INVESTIGATION STEPS ===

Follow these steps in order. Do not skip steps.

STEP 1 — Check for existing PRs (MANDATORY FIRST STEP)

  EXISTING=$(gh pr list --repo ${GITOPS_REPO} --state open \
    --json number,headRefName \
    --jq ".[] | select(.headRefName == \"fix/k8sgpt-${FINDING_FINGERPRINT}\") | .number")

  If EXISTING is non-empty:
  - Add a comment to the existing PR with your updated findings
  - This counts as your one action for this invocation — do NOT open a new PR
  - Exit after commenting

STEP 2 — Inspect the resource

  kubectl describe ${FINDING_KIND} ${FINDING_NAME} -n ${FINDING_NAMESPACE}
  kubectl get events -n ${FINDING_NAMESPACE} \
    --field-selector involvedObject.name=${FINDING_NAME} \
    --sort-by='.lastTimestamp'

  If the resource is a Pod, also run:
  kubectl logs ${FINDING_NAME} -n ${FINDING_NAMESPACE} --previous --tail=100 2>/dev/null || true
  kubectl logs ${FINDING_NAME} -n ${FINDING_NAMESPACE} --tail=100

STEP 3 — Check related resources

  Based on the kind and parent, inspect the owning resource:
  - If Pod: check the owning Deployment/DaemonSet/StatefulSet
  - If Service: check Endpoints and backing pods
  - If PersistentVolumeClaim: check the PV and StorageClass
  - Check relevant Events more broadly if the resource-scoped events are insufficient

STEP 4 — Run k8sgpt for deeper analysis

  k8sgpt analyze --filter ${FINDING_KIND} --namespace ${FINDING_NAMESPACE} --explain

STEP 5 — Locate the GitOps manifests

  Search /workspace/repo/${GITOPS_MANIFEST_ROOT}/ for the resource:
  grep -r "${FINDING_NAME}" /workspace/repo/${GITOPS_MANIFEST_ROOT}/ --include="*.yaml" -l
  grep -r "${FINDING_NAMESPACE}" /workspace/repo/${GITOPS_MANIFEST_ROOT}/ --include="*.yaml" -l

  Read the relevant HelmRelease, Kustomization, and values files.
  Use yq and jq to parse YAML/JSON as needed.

STEP 6 — Understand the Flux/Helm state

  flux get all -n ${FINDING_NAMESPACE}
  helm list -n ${FINDING_NAMESPACE}

  Search for HelmReleases and Kustomizations in the namespace. The HelmRelease name
  may differ from ${FINDING_PARENT} — look it up from the manifest files found in
  Step 5 or by listing:
  kubectl get helmreleases -n ${FINDING_NAMESPACE}
  kubectl get kustomizations -n ${FINDING_NAMESPACE}

  Once the HelmRelease name is known:
  flux logs --follow=false -n ${FINDING_NAMESPACE} --kind=HelmRelease --name=<release-name>
  helm get values <release-name> -n ${FINDING_NAMESPACE}

STEP 7 — Determine root cause

  Based on all evidence gathered, state clearly:
  1. What is broken and why
  2. What change in the GitOps repo would fix it
  3. How confident you are (high / medium / low)
  4. Whether the fix is safe to apply without human review

STEP 8 — Validate your proposed change

  Before creating the PR, validate any modified manifests:
  kubeconform -strict -ignore-missing-schemas <modified-file>
  kustomize build <overlay-path> | kubeconform -strict -ignore-missing-schemas -

STEP 9 — Open a pull request

  Branch name: fix/k8sgpt-${FINDING_FINGERPRINT}
  Base branch: main

  git -C /workspace/repo checkout -b fix/k8sgpt-${FINDING_FINGERPRINT}
  # make your changes
  git -C /workspace/repo add <changed files>
  git -C /workspace/repo commit -m "fix(${FINDING_KIND}/${FINDING_PARENT}): <concise description>"
  git -C /workspace/repo push origin fix/k8sgpt-${FINDING_FINGERPRINT}

  gh pr create \
    --repo ${GITOPS_REPO} \
    --base main \
    --head fix/k8sgpt-${FINDING_FINGERPRINT} \
    --title "fix(${FINDING_KIND}/${FINDING_PARENT}): <concise description>" \
    --body "<see PR body format below>"

=== PR BODY FORMAT ===

## Summary

Brief description of the problem and the fix.

## Finding

- **Kind:** ${FINDING_KIND}
- **Resource:** ${FINDING_NAME}
- **Namespace:** ${FINDING_NAMESPACE}
- **Parent:** ${FINDING_PARENT}
- **k8sgpt fingerprint:** \`${FINDING_FINGERPRINT}\`

## Evidence

What you observed during investigation (kubectl describe output, logs, events).
Include specific error messages and relevant context.

## Root Cause

Your assessment of why this is happening.

## Fix

What this PR changes and why it fixes the problem.

## Confidence

high / medium / low — and why.

## Notes

Any caveats, follow-up items, or things a human reviewer should check.

---
*Opened automatically by mendabot*

=== HARD RULES ===

These are non-negotiable. Violating any of them is an error.

1. NEVER commit directly to main — always use the fix/k8sgpt-${FINDING_FINGERPRINT} branch
2. NEVER create, read, modify, or reference Kubernetes Secrets in the GitOps repo
3. Exactly ONE of these two outcomes must occur per invocation:
   a. If an existing PR was found in Step 1: comment on it and exit. Do not open a new PR.
   b. If no existing PR was found: open exactly one PR. Not zero, not two.
4. If you cannot determine a safe fix with medium or high confidence:
   - Still open the PR (satisfying Rule 3b)
   - Leave the code unchanged
   - Fill the PR body with your full investigation findings
   - Add the label "needs-human-review" to the PR:
     gh pr edit <url> --add-label "needs-human-review"
5. The PR body must always include the fingerprint so humans can correlate it to the Result CRD
6. Do not install additional tools or modify system files
7. Do not make API calls to external services other than GitHub

=== DECISION TREE ===

Existing PR found (Step 1) → comment on it → exit (Rule 3a satisfied)
No existing PR AND fix identified with high/medium confidence → open fix PR (Rule 3b satisfied)
No existing PR AND fix is low confidence or unclear → open investigation-report PR with label needs-human-review (Rule 3b satisfied)
```

---

## 3. Environment Variable Substitution

The prompt references these variables which must all be set before `envsubst` runs:

| Variable | Set by | Example |
|---|---|---|
| `FINDING_KIND` | Watcher (Job env) | `Pod` |
| `FINDING_NAME` | Watcher (Job env) | `my-deployment-abc12` (plain name, no namespace/) |
| `FINDING_NAMESPACE` | Watcher (Job env) | `default` |
| `FINDING_PARENT` | Watcher (Job env) | `my-deployment` |
| `FINDING_FINGERPRINT` | Watcher (Job env) | `a3f9c2b14d8e...` (64 chars) |
| `FINDING_ERRORS` | Watcher (Job env) | `[{"text":"CrashLoopBackOff"}]` |
| `FINDING_DETAILS` | Watcher (Job env) | LLM-generated explanation |
| `GITOPS_REPO` | Watcher (Job env) | `lenaxia/talos-ops-prod` |
| `GITOPS_MANIFEST_ROOT` | Watcher (Job env) | `kubernetes` |

The entrypoint script uses a restricted variable list with `envsubst` to prevent
corruption of `FINDING_ERRORS` or `FINDING_DETAILS` content that may contain `$` signs:

```bash
VARS='${FINDING_KIND}${FINDING_NAME}${FINDING_NAMESPACE}${FINDING_PARENT}${FINDING_FINGERPRINT}${FINDING_ERRORS}${FINDING_DETAILS}${GITOPS_REPO}${GITOPS_MANIFEST_ROOT}'
envsubst "$VARS" < /prompt/prompt.txt > /tmp/rendered-prompt.txt
exec opencode run --file /tmp/rendered-prompt.txt
```

`opencode run --file <path>` passes the rendered prompt via a file path. This avoids
shell word-splitting and quoting issues that would occur if the prompt were passed as a
CLI argument with `"$(cat ...)"`.

**PR deduplication note:** Step 1 uses `gh pr list --json ... --jq` to filter by exact
branch name (`headRefName`), which is more reliable than `--search` text search. However,
after a Job's `ttlSecondsAfterFinished` (24h) expires, the Job is deleted and
`IsAlreadyExists` no longer prevents re-dispatch. If the watcher restarts after the Job
TTL, the agent will be re-dispatched — Step 1's branch-name check is the only guard
against opening a duplicate PR in this window. This is the designed and acceptable
behaviour.

---

## 4. Prompt Versioning

The prompt is stored in the `opencode-prompt` ConfigMap. When the prompt is updated:

1. Update `configmap-prompt.yaml`
2. Update the `Version` in this LLD
3. Write a worklog entry explaining what changed and why
4. New Jobs will automatically pick up the new prompt (ConfigMap is mounted at runtime)
5. In-flight Jobs are unaffected — they already have the prompt rendered

---

## 5. Prompt Tuning Guidelines

When tuning the prompt based on observed agent behaviour:

**If the agent opens duplicate PRs:**
- Strengthen the Step 1 language
- The deduplication uses `gh pr list --json --jq` filtering by exact `headRefName` value
  (`fix/k8sgpt-${FINDING_FINGERPRINT}`). If duplicates occur, verify the branch name
  pattern is correct and that `--state open` is covering the expected window.

**If the agent makes overly aggressive changes:**
- Add specific constraints to the Hard Rules section
- Lower the confidence threshold wording

**If the agent produces investigation-only PRs too often:**
- Review the confidence guidance
- Ensure evidence-gathering steps are thorough enough to reach a conclusion

**If the agent times out (hits 15 min deadline):**
- The investigation steps may be too broad
- Consider constraining the search scope in Step 5

**Never shorten the hard rules section.** If anything, add to it based on observed failures.
