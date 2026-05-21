PREFIX ?= /usr/local
BINDIR ?= $(PREFIX)/bin
BIN ?= bin/mdf
DISTDIR ?= dist
VERSION ?= $(shell tag=$$(git describe --tags --exact-match --match 'v[0-9]*.[0-9]*.[0-9]*' 2>/dev/null || true); if [ -n "$$tag" ]; then printf '%s\n' "$${tag#v}"; else printf '0.0.0\n'; fi)
RELEASE_MATRIX := \
	linux/amd64 \
	linux/arm64 \
	linux/arm/v7 \
	freebsd/amd64 \
	freebsd/arm64 \
	freebsd/arm/v7 \
	darwin/amd64 \
	darwin/arm64

.DEFAULT_GOAL := help

.PHONY: help build install release clean

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*## "}; /^[a-zA-Z0-9_.-]+:.*## / {printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: $(BIN) ## Build the local development binary under bin/

$(BIN):
	@mkdir -p $(dir $(BIN))
	CGO_ENABLED=0 go build -o $(BIN) -trimpath -buildvcs=true -ldflags '-s -w' ./cmd/mdf/

install: $(BIN) ## Install the local development binary
	@mkdir -p $(BINDIR)
	install -m 0755 $(BIN) $(BINDIR)/mdf

release: ## Build cross-platform release zip archives under dist/
	@set -eu; \
	if ! command -v zip >/dev/null 2>&1; then \
		echo "zip not found in PATH" >&2; \
		exit 1; \
	fi; \
	supported="$$(go tool dist list)"; \
	rm -rf "$(DISTDIR)"; \
	mkdir -p "$(DISTDIR)/stage"; \
	for target in $(RELEASE_MATRIX); do \
		os=$${target%%/*}; \
		rest=$${target#*/}; \
		goarch=$${rest%%/*}; \
		variant=""; \
		if [ "$$rest" != "$$goarch" ]; then \
			variant=$${rest#*/}; \
		fi; \
		if ! printf '%s\n' "$$supported" | grep -qx "$$os/$$goarch"; then \
			echo "skipping unsupported target $$os/$$goarch"; \
			continue; \
		fi; \
		pkg_arch=$$goarch; \
		goarm=""; \
		if [ "$$goarch" = "arm" ] && [ "$$variant" = "v7" ]; then \
			pkg_arch=armhf; \
			goarm=7; \
		fi; \
		base="mdf-$(VERSION)-$$os-$$pkg_arch"; \
		stage_dir="$(DISTDIR)/stage/$$base"; \
		archive_path="$(DISTDIR)/$$base.zip"; \
		echo "building $$archive_path"; \
		rm -rf "$$stage_dir" "$$archive_path"; \
		mkdir -p "$$stage_dir/bin" "$$stage_dir/share/mdf"; \
		env GOOS="$$os" GOARCH="$$goarch" GOARM="$$goarm" CGO_ENABLED=0 \
			go build -trimpath -buildvcs=true -ldflags '-s -w' -o "$$stage_dir/bin/mdf" ./cmd/mdf/; \
		cp LICENSE README.md "$$stage_dir/share/mdf/"; \
		( cd "$(DISTDIR)/stage" && zip -qr "../$$base.zip" "$$base" ); \
	done; \
	rm -rf "$(DISTDIR)/stage"

clean: ## Remove build and release artifacts
	go clean ./...
	rm -rf bin "$(DISTDIR)"
