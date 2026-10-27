SHELL := /bin/bash

BINARY    := thumb
MODULE    := github.com/thomaslaurenson/thumb
VERSION   := $(shell git describe --tags --always --dirty --match 'v*' 2>/dev/null || echo dev)
GOIMPORTS := go run golang.org/x/tools/cmd/goimports@latest -local $(MODULE)

##@ BUILD

.PHONY: help
help: ## Show this help message
	@awk 'BEGIN {FS = ":.*?## "} /^##@ / {printf "\n%s\n", substr($$0, 5)} \
		/^[a-zA-Z_-]+:.*## / {printf "  %-18s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

.PHONY: build
build: ## Build the binary into dist/
	go build -ldflags="-s -w -X $(MODULE)/cmd.Version=$(VERSION)" -o dist/$(BINARY) .

.PHONY: install
install: build ## Copy the binary into ~/.local/bin
	install -D -m 0755 dist/$(BINARY) $(HOME)/.local/bin/$(BINARY)

.PHONY: snapshot
snapshot: ## Build snapshot binaries with goreleaser
	goreleaser build --snapshot --clean

##@ TEST

.PHONY: test
test: ## Run the tests with the race detector
	go test -race -count=1 ./...

.PHONY: test_integration
test_integration: ## Run the integration tests too; never run in CI
	go test -race -count=1 -tags=integration ./...

.PHONY: test_coverage
test_coverage: ## Run the tests with a coverage report over internal/
	go test -race -count=1 -tags=integration -coverpkg=./internal/... -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out
	rm coverage.out

##@ LINT

.PHONY: format
format: ## Format and group imports
	$(GOIMPORTS) -w .

.PHONY: check_format
check_format: ## Fail if any file needs formatting
	@out="$$($(GOIMPORTS) -l .)"; test -z "$$out" || { printf '%s\n' "$$out"; exit 1; }

.PHONY: check_mod
check_mod: ## Fail if go.mod or go.sum are not tidy
	go mod tidy && git diff --exit-code go.mod go.sum

.PHONY: vet
vet: ## Run go vet, including integration-tagged files
	go vet ./...
	go vet -tags=integration ./...

.PHONY: check_cross
check_cross: ## Type-check for windows and darwin
	GOOS=windows go vet ./...
	GOOS=darwin go vet ./...

.PHONY: check_embed
check_embed: ## Fail if the embedded site templates do not parse
	go test -count=1 -run TestEmbeddedTemplatesParse ./internal/site

.PHONY: vuln
vuln: ## Scan for reachable vulnerabilities
	go run golang.org/x/vuln/cmd/govulncheck@latest ./...

##@ GET

.PHONY: get_version
get_version: ## Print the version build would stamp
	@echo "$(VERSION)"

.PHONY: get_changelog
get_changelog: ## Print release notes for TAG to stdout (TAG=v1.0.0)
	@tag="$(TAG)"; tag="$${tag#v}"; \
	if [[ -z "$$tag" ]]; then \
	  printf 'get_changelog: TAG is empty; pass TAG=v1.0.0\n' >&2; \
	  exit 1; \
	fi; \
	notes="$$(awk -v tag="$$tag" ' \
	  /^## / { if (found) exit; if (index($$0,"## "tag" ")==1 || $$0=="## "tag) found=1; next } \
	  found { lines[n++]=$$0 } \
	  END { \
	    s=0; while (s<n && lines[s]~/^[[:space:]]*$$/) s++; \
	    e=n-1; while (e>=s && lines[e]~/^[[:space:]]*$$/) e--; \
	    for (i=s;i<=e;i++) print lines[i] \
	  }' CHANGELOG.md)"; \
	if [[ -z "$$notes" ]]; then \
	  printf 'get_changelog: no CHANGELOG entry for %s\n' "$$tag" >&2; \
	  exit 1; \
	fi; \
	printf '%s\n' "$$notes"

##@ CI

.PHONY: check_all
check_all: check_format check_mod vet check_cross check_embed ## Run every static check

.PHONY: ci
ci: check_all test ## Run the checks CI runs

.PHONY: clean
clean: ## Remove build and release artefacts
	rm -rf dist/ coverage.out install.sh install.ps1 checksums.txt checksums.txt.sigstore.json
