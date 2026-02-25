# mendabot — developer convenience targets
# All targets are intended to be run from the repository root.

WATCHER_IMAGE ?= mendabot-watcher:dev
AGENT_IMAGE   ?= mendabot-agent:dev
KIND_CLUSTER  ?= mendabot-dev

.PHONY: build test lint lint-full lint-security lint-security-report \
        lint-secrets install-hooks \
        scan-watcher scan-agent \
        generate docs-helm docs-crd docs-check \
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

## lint-full: run golangci-lint (superset of go vet, includes gosec, staticcheck, etc.)
lint-full:
	golangci-lint run --config .golangci.yml ./...

## lint-secrets: scan staged + tracked files for secrets using gitleaks
lint-secrets:
	gitleaks detect --config .gitleaks.toml --redact --no-banner

## install-hooks: symlink git hooks from scripts/hooks/ into .git/hooks/
install-hooks:
	@mkdir -p .git/hooks
	@ln -sf "$(CURDIR)/scripts/hooks/pre-commit" .git/hooks/pre-commit
	@echo "Git hooks installed. Run 'make install-hooks' after cloning."

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

## generate: run controller-gen to regenerate CRD manifests from kubebuilder markers
generate:
	@which controller-gen > /dev/null 2>&1 || go install sigs.k8s.io/controller-tools/cmd/controller-gen@v0.19.0
	controller-gen crd:generateEmbeddedObjectMeta=true paths=./api/... output:crd:artifacts:config=charts/mendabot/crds
	cp charts/mendabot/crds/remediation.mendabot.io_remediationjobs.yaml testdata/crds/remediationjob_crd.yaml

## docs-helm: regenerate charts/mendabot/README.md from values.yaml and README.md.gotmpl
docs-helm:
	helm-docs --chart-search-root charts/ --template-files README.md.gotmpl

## docs-crd: generate docs/api-reference.md from Go types in api/v1alpha1/
docs-crd:
	crd-ref-docs \
		--source-path api/v1alpha1 \
		--config docs/crd-ref-docs-config.yaml \
		--renderer=markdown \
		--output-path docs/api-reference.md

## docs: regenerate all auto-generated documentation
docs: docs-helm docs-crd

## docs-check: fail if any generated docs are out of date (used in CI)
docs-check:
	@$(MAKE) docs generate
	@git diff --exit-code \
		charts/mendabot/README.md \
		docs/api-reference.md \
		charts/mendabot/crds/ \
		testdata/crds/ \
		|| (echo "\nERROR: generated docs are out of date. Run 'make docs generate' locally and commit the results." && exit 1)
