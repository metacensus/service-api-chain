.PHONY: help build test test-race test-integration test-artifact \
        fmt fmt-check vet tidy-check golangci vuln lint check docker-build docker-build-chaincode \
        release release-major release-minor release-patch latest

.DEFAULT_GOAL := help

IMAGE           ?= metacensus/service-api-chain
CHAINCODE       ?= metacensus/service-api-chain-chaincode
IMAGE_TAG       ?= dev

# Read out of go.mod so no copy can drift from what CI's setup-go uses.
GOTOOLCHAIN_PIN ?= $(shell awk '/^toolchain /{t=$$2} /^go /{if (g == "") g = "go" $$2} END{print (t != "" ? t : g)}' go.mod)
ifeq ($(GOTOOLCHAIN_PIN),)
$(error could not read the Go toolchain from go.mod; refusing to run unpinned)
endif
# A go.work above the checkout would lift the pins; ignore it.
export GOTOOLCHAIN := $(GOTOOLCHAIN_PIN)
export GOWORK := off

# Tools run at a pinned version rather than whatever is installed: a linter
# built with an older Go refuses a newer module, and CI must agree with a laptop.
GOLANGCI_LINT := github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0
GOVULNCHECK   := golang.org/x/vuln/cmd/govulncheck@v1.8.0

help:
	@awk -F' — ' '/^## /{ sub(/^## /, ""); printf "  make %-22s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

## build — compile both binaries into bin/
build:
	go build -trimpath -o bin/service ./cmd/service
	go build -trimpath -o bin/chaincode ./cmd/chaincode

## test — unit tests
test:
	go test ./...

## test-race — unit tests under the race detector
test-race:
	go test -race -count=1 ./...

## test-integration — the gateway and the store.Store conformance suite against Microfab (needs Docker)
test-integration:
	go test -tags=integration -race -count=1 -timeout 15m ./...

## test-artifact — the built images against Microfab (needs Docker; SERVICE_IMAGE and CHAINCODE_IMAGE, full refs, skip the build)
test-artifact: $(if $(SERVICE_IMAGE),,docker-build docker-build-chaincode)
	SERVICE_IMAGE=$(or $(SERVICE_IMAGE),$(IMAGE):$(IMAGE_TAG)) CHAINCODE_IMAGE=$(or $(CHAINCODE_IMAGE),$(CHAINCODE):$(IMAGE_TAG)) \
		go test -tags=artifact -count=1 -timeout 15m ./integration/...

## fmt — rewrite with gofmt
fmt:
	gofmt -w .

## fmt-check — fail if gofmt would rewrite anything
fmt-check:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt would rewrite:"; echo "$$unformatted"; exit 1; \
	fi

## vet — go vet, integration- and artifact-tagged code included
vet:
	go vet ./...
	go vet -tags=integration ./...
	go vet -tags=artifact ./...

## tidy-check — fail if go mod tidy would change go.mod or go.sum
tidy-check:
	go mod tidy -diff

## golangci — golangci-lint, from .golangci.yml
golangci:
	go run $(GOLANGCI_LINT) run ./...

## vuln — govulncheck over the module's dependencies
vuln:
	go run $(GOVULNCHECK) ./...

## lint — formatting, vet, module freshness and golangci-lint
lint: fmt-check vet tidy-check golangci

## check — lint, race tests and both integration suites
check: lint test-race test-integration test-artifact

## docker-build — build the API image for this machine's arch
docker-build:
	docker build --target service -t $(IMAGE):$(IMAGE_TAG) .

## docker-build-chaincode — build the chaincode image for this machine's arch
docker-build-chaincode:
	docker build --target chaincode -t $(CHAINCODE):$(IMAGE_TAG) .

## release — tag and push VERSION=x.y.z or the next TYPE=major|minor|patch; prompts unless YES=1
release:
	@set -e; \
	VERSION=$$(./scripts/version.sh "$(VERSION)" "$(TYPE)"); \
	TAG="v$$VERSION"; \
	if git rev-parse "$$TAG" >/dev/null 2>&1; then \
		echo "Error: Tag $$TAG already exists"; \
		exit 1; \
	fi; \
	if git ls-remote --exit-code --tags origin "refs/tags/$$TAG" >/dev/null 2>&1; then \
		echo "Error: Tag $$TAG already exists on origin"; \
		exit 1; \
	fi; \
	if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty; commit or clean it before releasing"; \
		exit 1; \
	fi; \
	git fetch -q origin main; \
	if ! git merge-base --is-ancestor HEAD origin/main; then \
		echo "Error: HEAD is not on origin/main; refusing to tag"; \
		exit 1; \
	fi; \
	echo "About to tag and push $$TAG, which publishes both images."; \
	if [ "$(YES)" != "1" ]; then \
		if [ -t 0 ]; then \
			printf "Proceed? [y/N] "; read -r reply; \
			case "$$reply" in y|Y|yes|YES) ;; *) echo "Aborted."; exit 1;; esac; \
		else \
			echo "Refusing to release non-interactively; pass YES=1 if you mean it."; \
			exit 1; \
		fi; \
	fi; \
	git tag -a "$$TAG" -m "Release $$VERSION" && \
	git push origin "$$TAG" && \
	echo "Released: $$TAG"

## release-major — release, bumping the major
release-major:
	@$(MAKE) release TYPE=major

## release-minor — release, bumping the minor
release-minor:
	@$(MAKE) release TYPE=minor

## release-patch — release, bumping the patch
release-patch:
	@$(MAKE) release TYPE=patch

## latest — print the most recent version tag
latest:
	@bash -c '. scripts/version.sh && v=$$(get_latest_version) && [ -n "$$v" ] && echo v$$v || true'
