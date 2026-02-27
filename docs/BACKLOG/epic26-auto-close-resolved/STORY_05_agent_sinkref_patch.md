# Story 05: Agent — Write SinkRef After gh pr create

## Status: Complete

## Goal

After the agent opens a PR (STEP 8 in the core prompt), instruct it to capture the PR
URL and number from `gh pr create --json url,number` output and write the structured
`SinkRef` fields back to the `RemediationJob`'s status subresource via
`kubectl patch`. This gives the watcher the structured data it needs to auto-close the
PR when the finding resolves.

## Background

The agent currently opens a PR in STEP 8 and the PR URL is later written to
`status.prRef` via a separate mechanism (or not at all in some flows). The `SinkRef`
struct (added by STORY_00) requires `Type`, `URL`, `Number`, and `Repo` for REST API
auto-close.

The agent is an AI model driven by the prompt in `charts/mendabot/files/prompts/core.txt`.
The correct mechanism to instruct the agent to write `SinkRef` is to add a new step to
the core prompt. No entrypoint shell script changes are needed — the agent calls
`gh pr create` as an AI tool call, not via a shell wrapper.

The `RemediationJob` name is deterministic: `mendabot-<FINDING_FINGERPRINT[:12]>`.
The agent already knows `FINDING_FINGERPRINT` from its env vars.
The agent already has `kubectl` available and has cluster write access for status.

## Acceptance Criteria

- [x] `charts/mendabot/files/prompts/core.txt` has a new STEP 9 after the `gh pr create`
      block in STEP 8
- [x] STEP 9 instructs the agent to run `gh pr create --json url,number` (if not already
      done) and `kubectl patch` the `RemediationJob` status with `sinkRef`
- [x] The `kubectl patch` uses the status subresource (`--subresource=status`)
- [x] The patch sets `status.sinkRef.type`, `status.sinkRef.url`,
      `status.sinkRef.number`, `status.sinkRef.repo`
- [x] STEP 9 is explicitly marked as **mandatory** (HARD RULE: if patch fails, log the
      error but do not fail the job — the PR is already open and must not be retracted)
- [x] STEP 9 also patches `status.prRef` for backwards compatibility with the existing
      `kubectl get rjobs` display column
- [x] The STEP 9 instructions cover both the "new PR opened" case and the "existing PR
      found" case (STEP 1 in the prompt finds an existing PR → agent comments on it →
      must still write `SinkRef` pointing to the existing PR)

## Implementation Notes

### core.txt STEP 9

Add the following after the `gh pr create` block and PR body format section, before
the `=== HARD RULES ===` section:

```
STEP 9 — Write SinkRef back to the RemediationJob (MANDATORY)

After STEP 8 completes (either a new PR was opened or an existing PR was found and
commented on), write the PR reference back to the RemediationJob status subresource.
This enables the watcher to auto-close the PR when the underlying issue resolves.

RJOB_NAME="mendabot-${FINDING_FINGERPRINT:0:12}"
RJOB_NAMESPACE="${AGENT_NAMESPACE}"

# Capture the PR URL and number. Use --json if you opened the PR in this step,
# or use gh pr view if you found an existing PR in STEP 1.
# PR_URL must be the full HTML URL (e.g. https://github.com/org/repo/pull/42).
# PR_NUMBER must be the integer number (e.g. 42).
# PR_REPO must be in "owner/repo" format (e.g. "lenaxia/talos-ops-prod").

# Example: if you ran gh pr create --json url,number earlier:
#   OUTPUT=$(gh pr create --repo ${GITOPS_REPO} ... --json url,number)
#   PR_URL=$(echo "$OUTPUT" | jq -r '.url')
#   PR_NUMBER=$(echo "$OUTPUT" | jq -r '.number')
# If you found an existing PR in STEP 1:
#   PR_URL=$(gh pr view --repo ${GITOPS_REPO} fix/mendabot-${FINDING_FINGERPRINT} --json url -q .url)
#   PR_NUMBER=$(gh pr view --repo ${GITOPS_REPO} fix/mendabot-${FINDING_FINGERPRINT} --json number -q .number)

PR_REPO="${GITOPS_REPO}"

kubectl patch remediationjob "${RJOB_NAME}" \
  --namespace "${RJOB_NAMESPACE}" \
  --subresource=status \
  --type=merge \
  --patch "{
    \"status\": {
      \"prRef\": \"${PR_URL}\",
      \"sinkRef\": {
        \"type\": \"pr\",
        \"url\": \"${PR_URL}\",
        \"number\": ${PR_NUMBER},
        \"repo\": \"${PR_REPO}\"
      }
    }
  }" || echo "WARNING: failed to write SinkRef to RemediationJob — PR is still open and valid"

If the kubectl patch fails, print a warning and continue. The PR is already open and
must not be retracted. The auto-close feature will not work for this run, but the
investigation result is preserved.
```

### Why not a shell wrapper?

The agent calls `gh pr create` as an AI tool call, not as a shell command in a wrapper
script. The entrypoint scripts (`entrypoint-opencode.sh`, `entrypoint-common.sh`) run
`opencode run` (or `claude run`) and then exit. There is no post-agent hook available.
The only reliable mechanism to instruct the agent to run post-PR steps is via the
prompt.

### Why --subresource=status?

The `RemediationJob` type uses `+kubebuilder:subresource:status`. Updates to `status`
fields via the main resource endpoint are silently rejected by the API server. The
agent must use `--subresource=status` for the patch to actually persist.

### Agent RBAC

The agent already has `update` on `remediationjobs/status` via the agent RBAC manifests
(added in the cascade prevention epic). Verify this before closing the story:

```bash
kubectl auth can-i update remediationjobs/status --as=system:serviceaccount:mendabot:mendabot-agent
```

If it returns `no`, add `remediationjobs/status` to the agent ClusterRole or Role.

### FINDING_FINGERPRINT length

`FINDING_FINGERPRINT` is a 64-character hex SHA256. The RemediationJob name is
`mendabot-<first 12 chars>`. The `${FINDING_FINGERPRINT:0:12}` bash substring
syntax extracts the first 12 characters. This must match the name computed in
`internal/provider/provider.go:433` (`"mendabot-" + fp[:12]`).

### AGENT_NAMESPACE env var

`AGENT_NAMESPACE` is already injected into the agent Job by `internal/jobbuilder/job.go`.
No new env vars are needed.

## Files Touched

| File | Change |
|------|--------|
| `charts/mendabot/files/prompts/core.txt` | Add STEP 9 after STEP 8 |

## Verification

After deploying, trigger a test finding. When the agent completes:

```bash
kubectl get rjob -n mendabot mendabot-<fp12> -o jsonpath='{.status.sinkRef}'
```

Should return a non-empty object with `type`, `url`, `number`, and `repo` populated.

Then delete/resolve the source resource that triggered the finding. Within a few
reconcile cycles, the PR should be auto-closed with a comment.
