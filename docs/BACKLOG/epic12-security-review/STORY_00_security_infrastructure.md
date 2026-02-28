# Story 00: Security and Pentest Infrastructure Setup

**Epic:** [epic12-security-review](README.md)
**Priority:** High
**Status:** Complete
**Estimated Effort:** 3 hours

---

## User Story

As a **mechanic developer**, I want a reproducible security toolchain and test cluster
in place before any engineering security stories are executed, so that every story in
this epic has a consistent, verifiable baseline to build against and test against.

---

## Background

The current state of the repository has no security scanning tooling and no reproducible
test cluster setup:

- There is no `Makefile` — all commands are run ad hoc.
- The two GitHub Actions workflows (`build-watcher.yaml`, `build-agent.yaml`) only build
  and push images on tag pushes; they do not run any security scans.
- There is no `gosec` run against the Go source code. Static analysis would catch
  issues like unsafe shell construction, weak crypto usage, and hardcoded credentials
  before they reach a running cluster.
- There is no `trivy` image scan of `ghcr.io/lenaxia/mechanic-watcher` or
  `ghcr.io/lenaxia/mechanic-agent`. A CVE in the base image (`debian:bookworm-slim` or
  `golang:1.23-bookworm`) would undermine all the code-level controls in stories 01–05.
- STORY_06 (pentest) requires a running cluster with a NetworkPolicy-aware CNI, but no
  script or configuration exists to provision one reproducibly. Each person running the
  pentest would set it up differently.

This story establishes the toolchain and test environment that all other stories in this
epic depend on or produce results for.

---

## Acceptance Criteria

- [ ] `Makefile` exists at the repository root with the targets defined in
      §Technical Implementation
- [ ] `make lint-security` runs `gosec` against `./...` and exits non-zero on findings
      of severity HIGH or CRITICAL
- [ ] `make scan-watcher` runs `trivy image` against the locally-built watcher image
      and exits non-zero on CRITICAL CVEs
- [ ] `make scan-agent` runs `trivy image` against the locally-built agent image
      and exits non-zero on CRITICAL CVEs
- [ ] `make dev-cluster` provisions a `kind` cluster named `mechanic-dev` with Cilium
      as the CNI (required for NetworkPolicy enforcement in STORY_06 TC-03)
- [ ] `make dev-cluster-destroy` tears down the `mechanic-dev` cluster cleanly
- [ ] `make scan-watcher` and `make scan-agent` are added as steps to
      `.github/workflows/build-watcher.yaml` and `.github/workflows/build-agent.yaml`
      respectively, running after the existing push step
- [ ] A `gosec` baseline report is written to `docs/BACKLOG/epic12-security-review/gosec-baseline.json`
      by running `make lint-security-report` so findings present before this epic are
      documented and not confused with new regressions
- [ ] `go test -timeout 30s -race ./...` continues to pass (this story adds no Go code)

---

## Technical Implementation

### New file: `Makefile`

```makefile
# mechanic — developer convenience targets
# All targets are intended to be run from the repository root.

WATCHER_IMAGE ?= mechanic-watcher:dev
AGENT_IMAGE   ?= mechanic-agent:dev
KIND_CLUSTER  ?= mechanic-dev

.PHONY: build test lint lint-security lint-security-report \
        scan-watcher scan-agent \
        dev-cluster dev-cluster-destroy help

## build: compile watcher binary
build:
	go build ./...

## test: run full test suite with race detector
test:
	go test -timeout 60s -race ./...

## lint: run go vet
lint:
	go vet ./...

## lint-security: run gosec static analysis (HIGH/CRITICAL fail the build)
lint-security:
	gosec -severity high -confidence medium -quiet ./...

## lint-security-report: write gosec findings to docs for baseline tracking
lint-security-report:
	gosec -fmt json -out docs/BACKLOG/epic12-security-review/gosec-baseline.json ./... || true
	@echo "Baseline written to docs/BACKLOG/epic12-security-review/gosec-baseline.json"

## docker-build-watcher: build the watcher image locally
docker-build-watcher:
	docker build -f docker/Dockerfile.watcher -t $(WATCHER_IMAGE) .

## docker-build-agent: build the agent image locally
docker-build-agent:
	docker build -f docker/Dockerfile.agent -t $(AGENT_IMAGE) .

## scan-watcher: trivy image scan of the watcher image (CRITICAL CVEs fail)
scan-watcher: docker-build-watcher
	trivy image --exit-code 1 --severity CRITICAL --quiet $(WATCHER_IMAGE)

## scan-agent: trivy image scan of the agent image (CRITICAL CVEs fail)
scan-agent: docker-build-agent
	trivy image --exit-code 1 --severity CRITICAL --quiet $(AGENT_IMAGE)

## dev-cluster: provision kind cluster with Cilium CNI for security testing
dev-cluster:
	@echo "Creating kind cluster $(KIND_CLUSTER) with Cilium CNI..."
	kind create cluster --name $(KIND_CLUSTER) --config hack/kind-config.yaml
	@echo "Installing Cilium..."
	helm repo add cilium https://helm.cilium.io/ --force-update
	helm install cilium cilium/cilium --version 1.15.5 \
		--namespace kube-system \
		--set kubeProxyReplacement=false
	@echo "Waiting for Cilium to be ready..."
	kubectl -n kube-system rollout status daemonset/cilium --timeout=120s
	@echo "Cluster $(KIND_CLUSTER) is ready."

## dev-cluster-destroy: tear down the kind test cluster
dev-cluster-destroy:
	kind delete cluster --name $(KIND_CLUSTER)

## help: list available targets
help:
	@grep -E '^## ' Makefile | sed 's/## /  /'
```

### New file: `hack/kind-config.yaml`

The kind config disables the default CNI so Cilium can be installed instead:

```yaml
# hack/kind-config.yaml
# kind cluster configuration for mechanic security testing.
# Disables the default kindnet CNI so Cilium can be installed,
# which is required for NetworkPolicy enforcement in STORY_06 TC-03.
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
networking:
  disableDefaultCNI: true
  podSubnet: "10.244.0.0/16"
nodes:
- role: control-plane
- role: worker
```

### Changes to `.github/workflows/build-watcher.yaml`

Add a `trivy` scan step after the existing `Build and push` step:

```yaml
- name: Scan watcher image for CVEs
  uses: aquasecurity/trivy-action@0.20.0
  with:
    image-ref: ghcr.io/lenaxia/mechanic-watcher:sha-${{ steps.sha.outputs.short }}
    format: table
    exit-code: '1'
    severity: CRITICAL
    ignore-unfixed: true
```

### Changes to `.github/workflows/build-agent.yaml`

Add the same step after the `Build and push` step:

```yaml
- name: Scan agent image for CVEs
  uses: aquasecurity/trivy-action@0.20.0
  with:
    image-ref: ghcr.io/lenaxia/mechanic-agent:sha-${{ steps.sha.outputs.short }}
    format: table
    exit-code: '1'
    severity: CRITICAL
    ignore-unfixed: true
```

### Tool versions

| Tool | Version | Install |
|------|---------|---------|
| `gosec` | v2.20.0 | `go install github.com/securego/gosec/v2/cmd/gosec@v2.20.0` |
| `trivy` | v0.51.x | `brew install trivy` / `apt install trivy` / GitHub release |
| `kind` | v0.23.x | `go install sigs.k8s.io/kind@v0.23.0` |
| `helm` | v3.x | already required by chart work |

These are developer workstation tools. They are not added as Go module dependencies.
The CI workflows use the official `aquasecurity/trivy-action` which pins its own version.

---

## Tasks

- [ ] Create `Makefile` at repository root with all targets listed above
- [ ] Create `hack/kind-config.yaml`
- [ ] Run `make lint-security-report` to produce `gosec-baseline.json` — this documents
      any pre-existing findings so they are not counted as regressions from this epic
- [ ] Run `make lint-security` — review any HIGH/CRITICAL findings; record known
      false-positives in a comment block at the top of `gosec-baseline.json`
- [ ] Run `make docker-build-watcher && make scan-watcher` — record any CRITICAL CVEs
      found in the baseline; update base images if any are fixable
- [ ] Run `make docker-build-agent && make scan-agent` — same
- [ ] Add trivy scan step to `.github/workflows/build-watcher.yaml`
- [ ] Add trivy scan step to `.github/workflows/build-agent.yaml`
- [ ] Run `make dev-cluster` and verify `kubectl get nodes` shows both nodes Ready
- [ ] Verify `kubectl -n kube-system get pods` shows Cilium daemonset pods Running
- [ ] Run `make dev-cluster-destroy` and verify cluster is removed

---

## Dependencies

**Depends on:** epic04-deploy (base kustomize directory exists), epic06-ci-cd (existing
GitHub Actions workflows this story extends)
**Blocks:** STORY_02 (security overlay dry-run needs `kubectl`), STORY_06 (pentest
requires `dev-cluster` with Cilium)

---

## Definition of Done

- [ ] `Makefile` exists and all targets run without error on a developer workstation
      with the required tools installed
- [ ] `gosec-baseline.json` exists in the epic directory and is committed to the repo
- [ ] Both GitHub Actions workflows include a trivy scan step that will fail the build
      on CRITICAL CVEs
- [ ] `make dev-cluster` produces a working kind cluster with Cilium enforcing
      NetworkPolicy (verified by `kubectl -n kube-system get pods` showing cilium pods Running)
- [ ] `make dev-cluster-destroy` cleans up completely
- [ ] No new Go code is introduced — this story is infrastructure only
