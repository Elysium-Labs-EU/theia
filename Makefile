.PHONY: help list dev build install test lint check-pubkey-sync check-file-size check-golangci-pin check-action-pins nilcheck crap govulncheck secrets leak-test clean docker-* test-docker-* changelog changelog-preview release release-prepare release-local fix setup
.DEFAULT_GOAL := help

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_DATE ?= $(shell date -u '+%Y-%m-%d %H:%M:%S UTC')
VERSION_PKG := github.com/Elysium-Labs-EU/theia/internal/buildinfo
COVERAGE_THRESHOLD ?= 60
LDFLAGS := -ldflags "-X '$(VERSION_PKG).Version=$(VERSION)' -X '$(VERSION_PKG).GitCommit=$(COMMIT)' -X '$(VERSION_PKG).BuildDate=$(BUILD_DATE)' -w -s"

BINARY_NAME=theia
GOBIN=./bin
INSTALL_PATH=~/.local/bin

# golangci-lint's cache defaults to one shared directory per user and keys
# entries on file content. Two worktrees of this repo hold identical sources,
# so their keys collide and one tree is served the other's stored findings,
# carrying the paths recorded when they were first analysed. Keeping the cache
# in the worktree makes collision impossible and needs no reaping, since it is
# removed with the tree that owns it. lefthook sets this inline too - it
# invokes golangci-lint directly, so this export never reaches it. Must be
# absolute; $(CURDIR) is.
export GOLANGCI_LINT_CACHE := $(CURDIR)/.cache/golangci-lint
# Single source of truth for the golangci-lint pin: lint, fix, the pre-commit
# hook (lefthook.yml) and the golangci-lint workflow all read this file so
# none of them can drift to whatever golangci-lint happens to resolve to on
# PATH.
GOLANGCI_LINT_VERSION := $(shell cat .golangci-lint-version)

setup: ## Install dev tools (golangci-lint, nilaway, go-crap, git-cliff) and git hooks
	@echo "Installing golangci-lint $(GOLANGCI_LINT_VERSION)..."
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "Installing nilaway (nil pointer static analysis)..."
	go install go.uber.org/nilaway/cmd/nilaway@latest
	@echo "Installing go-crap (CRAP score analysis)..."
	go install github.com/padiazg/go-crap@latest
	@echo "Installing govulncheck..."
	go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "Installing gitleaks..."
	go install github.com/zricethezav/gitleaks/v8@latest
	@echo "Installing git-cliff (changelog generation)..."
	@command -v cargo >/dev/null 2>&1 && cargo install git-cliff || echo "cargo not found — install git-cliff manually: https://git-cliff.org/docs/installation"
	@echo "Setup complete."

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-28s\033[0m %s\n", $$1, $$2}' | sort

list: help ## List all available commands

dev: ## Run theia locally
	@echo "Running theia in development mode..."
	go run . daemon

build: ## Build binary with version info
	@echo "Building theia $(VERSION)..."
	@mkdir -p $(GOBIN)
	CGO_ENABLED=0 go build $(LDFLAGS) -o $(GOBIN)/$(BINARY_NAME) .
	@echo "Binary built: $(GOBIN)/$(BINARY_NAME)"

install: build ## Install to ~/.local/bin
	@echo "Installing to $(INSTALL_PATH)..."
	@mkdir -p $(INSTALL_PATH)
	cp $(GOBIN)/$(BINARY_NAME) $(INSTALL_PATH)/
	@echo "Installed! Run 'theia --help' to get started"

test: ## Run tests
	@echo "Running tests..."
	go test . ./cmd ./internal/... ./database/... -race -count=2

test-coverage: ## Get test coverage
	@echo "Getting test coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out

test-coverage-check: ## Fail if total coverage is below COVERAGE_THRESHOLD
	@go test -coverprofile=coverage.out ./... -covermode=atomic -count=1 2>&1 | grep -v "^?" || true
	@total=$$(go tool cover -func=coverage.out | awk '/^total:/{gsub(/%/,""); print $$3}'); \
	echo "Total coverage: $${total}%"; \
	awk -v total="$${total}" -v threshold="$(COVERAGE_THRESHOLD)" \
		'BEGIN { if (total+0 < threshold+0) { print "Coverage " total "% below threshold " threshold "%"; exit 1 } }'

lint: check-pubkey-sync ## Run all linters (always the pinned $(GOLANGCI_LINT_VERSION), regardless of what's on PATH)
	@echo "Running linters..."
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) run --timeout=5m

check-pubkey-sync: ## Fail if the release signing public key in cmd/update.go and install.sh have drifted apart
	@sh scripts/check-pubkey-sync.sh

check-file-size: ## Fail if a changed file vs origin/main is oversized or an LFS pointer
	@bash scripts/check-file-size.sh

check-golangci-pin: ## Fail if the golangci-lint version has drifted from .golangci-lint-version anywhere
	@bash scripts/check-golangci-pin.sh

check-action-pins: ## Fail if a GitHub Actions workflow references an action by floating tag instead of SHA
	@bash scripts/check-action-pins.sh

nilcheck: ## Static nil-pointer safety analysis (requires: go install go.uber.org/nilaway/cmd/nilaway@latest)
	@echo "Running nilaway nil pointer analysis..."
	@command -v nilaway >/dev/null 2>&1 || { echo "nilaway not found. Run: make setup"; exit 1; }
	nilaway ./...

crap: test-coverage-check ## Change-scoped CRAP gate (fails only on functions this change modified; requires: go install github.com/padiazg/go-crap@latest)
	@echo "Running change-scoped go-crap CRAP gate..."
	@command -v go-crap >/dev/null 2>&1 || { echo "go-crap not found. Run: make setup"; exit 1; }
	bash scripts/go-crap-gate.sh .

crap-report: ## Full whole-repo CRAP report (informational, non-blocking)
	@echo "Running whole-repo go-crap CRAP report..."
	@command -v go-crap >/dev/null 2>&1 || { echo "go-crap not found. Run: make setup"; exit 1; }
	go-crap scan

govulncheck: ## Reachability-aware vulnerability scan (complements OSV-Scanner's lockfile-only scan)
	@command -v govulncheck >/dev/null 2>&1 || { echo "govulncheck not found. Run: make setup"; exit 1; }
	govulncheck ./...

secrets: ## Scan full git history + working tree for committed secrets
	@command -v gitleaks >/dev/null 2>&1 || { echo "gitleaks not found. Run: make setup"; exit 1; }
	gitleaks detect --source . --no-banner --redact

leak-test: ## Run tests with goroutine leak detection
	@echo "Running tests with goroutine leak detection..."
	go test ./... -count=1 -timeout=60s -v 2>&1 | grep -E "(PASS|FAIL|leak|goroutine)" || true

fix: ## Fix go formatting
	go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION) fmt
	go run golang.org/x/tools/go/analysis/passes/fieldalignment/cmd/fieldalignment@latest -fix ./...

ci: ## Run all CI checks locally (runs all, reports all failures)
	@failed=0; \
	$(MAKE) test || failed=1; \
	$(MAKE) lint || failed=1; \
	$(MAKE) nilcheck || failed=1; \
	$(MAKE) crap || failed=1; \
	$(MAKE) govulncheck || failed=1; \
	$(MAKE) secrets || failed=1; \
	$(MAKE) check-file-size || failed=1; \
	$(MAKE) check-golangci-pin || failed=1; \
	$(MAKE) check-action-pins || failed=1; \
	if [ $$failed -ne 0 ]; then echo "CI checks FAILED"; exit 1; fi; \
	echo "All CI checks passed!"

docker-local: ## Test with local Docker setup
	@echo "Starting local Docker test environment..."
	@mkdir -p test-files-local/nginx-logs
	@sh -c 'docker compose -f test-files-local/docker-compose.yml up --build'

docker-local-down:  ## Tear down local Docker setup
	docker compose -f test-files-local/docker-compose.yml down

docker-local-logs: ## Tail logs local Docker setup
	docker compose -f test-files-local/docker-compose.yml logs -f

docker-vps: ## Test install.sh in VPS simulator
	@echo "Starting VPS simulator..."
	@echo "Once started, run: make docker-vps-test"
	@sh -c 'docker compose -f test-files-vps/docker-compose.yml up --build -d'

docker-vps-down: ## Tear down VPS simulator
	docker compose -f test-files-vps/docker-compose.yml down

docker-vps-test: ## Run install.sh in VPS simulator
	@echo "Testing install.sh in VPS simulator..."
	docker exec -it vps-test-theia bash -c "cd /test-scripts && bash install.sh"

docker-vps-shell: ## Open shell in VPS simulator
	@echo "Opening shell in VPS simulator..."
	docker exec -it vps-test-theia bash

docker-vps-status: ## Check theia service status in VPS simulator
	@echo "Checking theia service status in VPS..."
	docker exec -it vps-test-theia systemctl status theia

docker-vps-logs: ## Follow theia logs in VPS simulator
	@echo "Following theia logs in VPS..."
	docker exec -it vps-test-theia journalctl -u theia -f

test-docker-build: ## Build Linux test Docker image
	docker build -f test-files/Dockerfile.test -t theia-test .

test-docker-linux: test-docker-build ## Run tests in Linux Docker container
	docker run --rm theia-test

test-docker-linux-verbose: test-docker-build ## Run tests in Linux Docker container with verbose output
	docker run --rm theia-test go test ./cmd ./internal/... -race -count=1 -v

test-docker-linux-single: test-docker-build ## Run a single test in Linux Docker container (TEST=TestName)
	docker run --rm theia-test go test ./cmd ./internal/... -race -count=1 -v -run $(TEST)

docker-clean: ## Clean up Docker resources
	@echo "Cleaning up Docker resources..."
	docker compose -f test-files-local/docker-compose.yml down -v 2>/dev/null || true
	docker compose -f test-files-vps/docker-compose.yml down -v 2>/dev/null || true
	docker rmi theia-test 2>/dev/null || true
	rm -rf test-files-local/nginx-logs

release-local: ## Build release binaries locally
	@echo "Building release binaries..."
	@mkdir -p dist
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/theia-linux-amd64 .
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/theia-linux-arm64 .
	cd dist && sha256sum theia-linux-* > sha256sums.txt
	@echo "Release binaries built in ./dist/"
	@ls -lh dist/

changelog: ## Generate CHANGELOG.md from git history
	@command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff not found. Install: https://git-cliff.org/docs/installation"; exit 1; }
	git cliff --output CHANGELOG.md

changelog-preview: ## Preview unreleased changes (does not write to file)
	@command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff not found. Install: https://git-cliff.org/docs/installation"; exit 1; }
	git cliff --unreleased

release-prepare: ## Regenerate the changelog for a tag and commit it locally (main is protected — push this branch and merge it via PR before running 'release')
	@if [ -z "$(TAG)" ]; then echo "Usage: make release-prepare TAG=v1.2.0"; exit 1; fi
	@command -v git-cliff >/dev/null 2>&1 || { echo "git-cliff not found. Install: https://git-cliff.org/docs/installation"; exit 1; }
	git cliff --tag $(TAG) --output CHANGELOG.md
	git add CHANGELOG.md
	git diff --cached --quiet CHANGELOG.md || git commit -m "chore: update changelog for $(TAG)"
	@echo "CHANGELOG.md updated for $(TAG). Push this branch, open a PR against main, and merge it before running 'make release TAG=$(TAG)'."

release: ## Tag and push a release (run on main after the release-prepare changelog PR is merged)
	@if [ -z "$(TAG)" ]; then echo "Usage: make release TAG=v1.2.0"; exit 1; fi
	git tag -a $(TAG) -m "Release $(TAG)"
	git push origin $(TAG)

pre-release: ## Tag and push a pre-release
	@if [ -z "$(TAG)" ]; then echo "Usage: make pre-release TAG=v1.2.0-rc.1"; exit 1; fi
	git tag -a $(TAG) -m "Pre-release $(TAG)"
	git push origin $(TAG)

test-install-local: release-local ## Build and test local binary install in VPS simulator
	@$(MAKE) docker-vps
	@sleep 5
	@echo "Copying binary to VPS..."
	docker cp dist/theia-linux-amd64 vps-test-theia:/usr/local/src/theia-local
	docker exec -it vps-test-theia ls -la /usr/local/src/theia-local
	@echo "Running install.sh..."
	docker exec -it vps-test-theia bash -c "cd /test-scripts && bash install.sh -y --local /usr/local/src/theia-local"
	docker exec -it vps-test-theia bash -c 'curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/v0.40.4/install.sh | bash && \. "$$HOME/.nvm/nvm.sh" && nvm install 24 && corepack enable pnpm'

test-install-remote: ## Test remote install.sh in VPS simulator
	@$(MAKE) docker-vps
	@sleep 5
	@echo "Running install.sh..."
	docker exec -it vps-test-theia bash -c "cd /test-scripts && bash install.sh"

clean: ## Remove build artifacts and clean Docker resources
	@echo "Cleaning..."
	rm -rf $(GOBIN) dist/ $(GOLANGCI_LINT_CACHE)
	@$(MAKE) docker-clean
	go clean
	@echo "Cleaned"