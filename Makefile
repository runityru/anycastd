# anycastd Makefile
#
# Standard make interface (modeled on mcp-netbox): CI and local dev call
# `make <target>` rather than raw Go / golangci-lint commands. Go version is
# pinned by go.mod (go 1.27.x). All recipes use tabs.

GO ?= go
GOARCH_LINT_VERSION := v1.19.0

# --- Image build variables (used by the container-image target) --------------
# IMAGE is the registry image name; VERSION is the default tag suffix; TAGS may
# be a space- or comma-separated list of full image tags; PLATFORMS is the
# multi-arch target list.
IMAGE ?= ghcr.io/runityru/anycastd
VERSION ?= local
PLATFORMS ?= linux/amd64,linux/arm64
TAGS ?= $(IMAGE):$(VERSION)
comma := ,
TAGS_SPACE := $(subst $(comma), ,$(TAGS))

# SNAPSHOT=false produces a real goreleaser release (used by release.yml); the
# default (true) produces a local snapshot build into dist/.
SNAPSHOT ?= true

.PHONY: test lint lint-golangci lint-gosec lint-vuln arch build container-image

## test runs the unit suite with the race detector and writes coverage.out.
## No hard coverage gate is enforced (coverage is informational only).
test:
	$(GO) test -race -coverprofile=coverage.out -count=1 ./...

## lint runs the full static-analysis suite (golangci + gosec + govulncheck + arch).
## gosec/govulncheck are surfaced here as part of the single interface; CI also
## runs them as a separate non-failing (continue-on-error) security step so they
## never hard-gate the build.
lint: lint-golangci lint-gosec lint-vuln arch

## lint-golangci runs golangci-lint with the repo configuration.
lint-golangci:
	golangci-lint run ./...

## lint-gosec runs the standalone Go security scanner (gosec v2.30.0).
## gosec v2.29.0 could not read Go 1.27 export data; v2.30.0 supports it.
lint-gosec:
	$(GO) run github.com/securego/gosec/v2/cmd/gosec@v2.30.0 -quiet ./...

## lint-vuln runs the Go vulnerability scanner (govulncheck v1.9.0) and blocks
## only on FIXABLE vulnerabilities. Known advisories with no upstream fix
## (e.g. GO-2026-4736, GoBGP NEXT_HOP DoS) are reported but do not block CI —
## they are tracked as accepted risks. Output is filtered via
## scripts/govulncheck_filter.py.
lint-vuln:
	$(GO) run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -format json ./... 2>/dev/null > /tmp/govulncheck.json || true
	python3 scripts/govulncheck_filter.py /tmp/govulncheck.json

## arch runs the architecture dependency check (go-arch-lint).
arch:
	$(GO) run github.com/fe3dback/go-arch-lint@$(GOARCH_LINT_VERSION) check

## build produces a unified dist/ artifact via goreleaser (snapshot by default;
## use `make build SNAPSHOT=false` for a real release).
build:
	goreleaser release --clean $(if $(filter false,$(SNAPSHOT)),,--snapshot)

## container-image builds the multi-arch Docker image from the goreleaser dist/
## artifact. Goreleaser names the dist dirs with the default microarch suffix
## (anycastd_linux_amd64_v1, anycastd_linux_arm64_v8.0), so each binary is copied
## to a plain root-level name that the Dockerfile maps via ARG TARGETARCH.
## Usage:
##   make container-image                 # build only
##   make container-image push=true       # build and push
## Variables: IMAGE, TAGS (space/comma list), PLATFORMS.
container-image:
	cp dist/anycastd_linux_amd64_v1/anycastd anycastd-linux-amd64
	cp dist/anycastd_linux_arm64_v8.0/anycastd anycastd-linux-arm64
	docker buildx build --platform $(PLATFORMS) $(if $(filter true,$(push)),--push,) $(foreach t,$(TAGS_SPACE),-t $(t)) .
