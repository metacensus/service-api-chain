.PHONY: help build test test-race test-integration lint tidy-check check \
        docker-build docker-build-chaincode \
        release release-major release-minor release-patch latest list delete-tag

.DEFAULT_GOAL := help

IMAGE           ?= metacensus/service-api-chain
CHAINCODE_IMAGE ?= metacensus/service-api-chain-chaincode
IMAGE_TAG       ?= dev

# Read from go.mod; an empty GOTOOLCHAIN is silently accepted, so refuse it.
GOTOOLCHAIN_PIN ?= $(shell awk '/^toolchain /{t=$$2} /^go /{if (g == "") g = "go" $$2} END{print (t != "" ? t : g)}' go.mod)
ifeq ($(GOTOOLCHAIN_PIN),)
$(error could not read the Go toolchain from go.mod; refusing to build unpinned)
endif
export GOTOOLCHAIN := $(GOTOOLCHAIN_PIN)

help:
	@awk -F' — ' '/^## /{ sub(/^## /, ""); printf "  make %-26s %s\n", $$1, $$2 }' $(MAKEFILE_LIST)

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

## test-integration — build both images (unless SKIP_DOCKER_BUILD=1) and exercise them on Microfab; needs Docker
test-integration: $(if $(SKIP_DOCKER_BUILD),,docker-build docker-build-chaincode)
	cd integration && SERVICE_IMAGE=$(IMAGE):$(IMAGE_TAG) CHAINCODE_IMAGE=$(CHAINCODE_IMAGE):$(IMAGE_TAG) \
		go test -tags=integration -count=1 -timeout 20m ./...

## lint — gofmt -l is empty, go vet, including the integration-tagged code
lint:
	@unformatted=$$(gofmt -l .); \
	if [ -n "$$unformatted" ]; then \
		echo "gofmt would rewrite:"; echo "$$unformatted"; exit 1; \
	fi
	go vet ./...
	cd integration && go vet -tags=integration ./...

## tidy-check — fail if go mod tidy would change go.mod/go.sum, here or in integration/
tidy-check:
	go mod tidy -diff
	cd integration && go mod tidy -diff

## check — lint, tidy-check, test-race: everything but integration
check: lint tidy-check test-race

## docker-build — build the API image for this machine's arch
docker-build:
	docker build -t $(IMAGE):$(IMAGE_TAG) .

## docker-build-chaincode — build the chaincode image for this machine's arch
docker-build-chaincode:
	docker build -f Dockerfile.chaincode -t $(CHAINCODE_IMAGE):$(IMAGE_TAG) .

## release — tag and push VERSION=x.y.z or the next TYPE=major|minor|patch; prompts unless YES=1
release: scripts/version.sh
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
	@git tag -l "v*" | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+$$" | sort -V | tail -1

## list — print every version tag
list:
	@git tag -l "v*" | grep -E "^v[0-9]+\.[0-9]+\.[0-9]+$$" | sort -V

## delete-tag — delete TAG=vX.Y.Z locally and on the remote
delete-tag:
	@if [ -z "$(TAG)" ]; then \
		echo "Usage: make delete-tag TAG=v1.2.3"; \
		exit 1; \
	fi; \
	git tag -d "$(TAG)" 2>/dev/null || true; \
	git push origin ":refs/tags/$(TAG)"
