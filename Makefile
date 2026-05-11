# Chimera Gateway — see docs/plans/makefile-plan.md and README.md
#
# clean:      removes $(CHIMERA_GATEWAY_BIN)[.exe], $(CHIMERA_INDEX_BIN)[.exe], $(DESKTOP_BIN), dist/ only.
# clean-all:  also removes bin/, packaging/qdrant-bundles/, packages/, node_modules/, .deps/, run/, logs/ (requires CONFIRM=1; runs clean first).
# clean-data: removes data/bifrost/, data/qdrant/, data/gateway/ — fresh BiFrost + Qdrant + gateway metrics state (requires CONFIRM=1).
#
# Gateway/indexer basenames (no .exe). Defaults must match scripts/chimera-names.sh.
CHIMERA_GATEWAY_BIN := chimera
CHIMERA_INDEX_BIN := chimera-index
CHIMERA_CMD_GATEWAY := ./cmd/$(CHIMERA_GATEWAY_BIN)
CHIMERA_CMD_INDEXER := ./cmd/$(CHIMERA_INDEX_BIN)

ifeq ($(OS),Windows_NT)
  # Same bash as scripts/*.sh (Git for Windows). MSYS2-only: set GITBASH, e.g.
  #   set "GITBASH=C:\msys64\usr\bin\bash.exe"
  ifeq ($(origin GITBASH),undefined)
    # Per-machine install first; then per-user (winget / default installer path).
    _GIT_BASH := $(wildcard $(ProgramW6432)/Git/bin/bash.exe)
    ifeq ($(_GIT_BASH),)
      _GIT_BASH := $(wildcard $(LOCALAPPDATA)/Programs/Git/bin/bash.exe)
    endif
    ifneq ($(_GIT_BASH),)
      GITBASH := "$(firstword $(_GIT_BASH))"
    else
      GITBASH := "$(ProgramW6432)/Git/bin/bash.exe"
    endif
  endif
  RACE_GATEWAY :=
  BIFROST_BIN := bin/bifrost-http.exe
  QDRANT_BIN := bin/qdrant.exe
  DESKTOP_BIN := porcelain-desktop.exe
else
  ifeq ($(origin GITBASH),undefined)
    GITBASH := bash
  endif
  RACE_GATEWAY := -race
  BIFROST_BIN := bin/bifrost-http
  QDRANT_BIN := bin/qdrant
  DESKTOP_BIN := porcelain-desktop
endif

# Desktop vet/test need CGO + WebKit (see desktop-install). Set SKIP_DESKTOP=1 to omit chimera desktop tag.
SKIP_DESKTOP ?=
ifeq ($(SKIP_DESKTOP),1)
  _DESKTOP_VET :=
  _DESKTOP_TEST :=
else
  _DESKTOP_VET := vet-desktop
  _DESKTOP_TEST := test-desktop
endif

# UP_STACK=0 starts background supervisor without Qdrant; default is full stack.
ifeq ($(UP_STACK),0)
  _BG_FLAGS :=
else
  _BG_FLAGS := --stack
endif

.PHONY: help up configure install chimera-install claudia-install clean clean-all clean-data fmt fmt-check logs \
	bash \
	build chimera-build claudia-build tokencount-file desktop-install desktop-build desktop-run run \
	chimera-run chimera-serve chimera-start chimera-stop chimera-status \
	claudia-run claudia-serve claudia-start claudia-stop claudia-status \
	indexer-build indexer-run indexer-install \
	catalog-free catalog-available config-provider-free-tier \
	release-install release-snapshot package \
	vet vet-module vet-desktop \
	test test-internal test-catalog-free test-catalog-available test-chimera test-claudia test-desktop \
	precommit

# One bash process (same as scripts/*.sh) so Win32 Make does not run cmd `echo`/printf per line (quotes + CreateProcess failures).
help:
	@$(GITBASH) scripts/print-make-help.sh

# --- Full stack onboarding (see docs/plans/makefile-plan.md) ---
up: configure install build desktop-run

configure:
	$(GITBASH) scripts/configure.sh

bash:
	$(GITBASH) -il

install:
	$(MAKE) chimera-install desktop-install

chimera-install:
	$(GITBASH) scripts/install.sh

claudia-install: chimera-install

chimera-start:
	$(GITBASH) scripts/chimera-start.sh $(_BG_FLAGS)

claudia-start: chimera-start

chimera-stop:
	$(GITBASH) scripts/chimera-stop.sh

claudia-stop: chimera-stop

chimera-status:
	$(GITBASH) scripts/chimera-status.sh

claudia-status: chimera-status

logs:
	$(GITBASH) scripts/logs.sh

clean:
	$(GITBASH) scripts/clean.sh

clean-all:
	$(GITBASH) scripts/clean-all-confirm.sh $(CONFIRM)
	$(MAKE) clean
	$(GITBASH) scripts/clean-all.sh

clean-data:
	$(GITBASH) scripts/clean-data.sh $(CONFIRM)

fmt:
	gofmt -w cmd internal

fmt-check:
	$(GITBASH) scripts/fmt-check.sh

vet: vet-module $(_DESKTOP_VET)

vet-module:
	go vet ./...

vet-desktop: export CGO_ENABLED := 1
vet-desktop:
	go vet -tags desktop $(CHIMERA_CMD_GATEWAY)

test: test-internal test-catalog-free test-catalog-available test-chimera $(_DESKTOP_TEST)

test-internal:
	go test ./internal/... $(RACE_GATEWAY) -count=1

test-catalog-free:
	go test ./cmd/catalog-write-free $(RACE_GATEWAY) -count=1

test-catalog-available:
	go test ./cmd/catalog-write-available $(RACE_GATEWAY) -count=1

test-chimera:
	go test $(CHIMERA_CMD_GATEWAY) $(RACE_GATEWAY) -count=1

test-claudia: test-chimera

test-desktop: export CGO_ENABLED := 1
test-desktop:
	go test -tags desktop $(CHIMERA_CMD_GATEWAY) $(RACE_GATEWAY) -count=1

precommit: fmt-check vet test

build:
	$(MAKE) chimera-build desktop-build

chimera-build:
	go build -o $(CHIMERA_GATEWAY_BIN) $(CHIMERA_CMD_GATEWAY)

claudia-build: chimera-build

# v0.2 workspace file indexer (see docs/plans/indexer.plan.md). Builds $(CHIMERA_INDEX_BIN)[.exe] in repo root.
indexer-build:
ifeq ($(OS),Windows_NT)
	go build -o $(CHIMERA_INDEX_BIN).exe $(CHIMERA_CMD_INDEXER)
else
	go build -o $(CHIMERA_INDEX_BIN) $(CHIMERA_CMD_INDEXER)
endif

indexer-run:
	go run $(CHIMERA_CMD_INDEXER) $(ARGS)

indexer-install:
	go install $(CHIMERA_CMD_INDEXER)

# Print bytes + cl100k_base + o200k_base token counts for a file (requires FILE=path).
tokencount-file:
ifeq ($(FILE),)
	$(error FILE is required, e.g. make tokencount-file FILE=temp/groq-request.json)
endif
	go run $(CHIMERA_CMD_GATEWAY) tokencount -f "$(FILE)"

# Fetch Groq rate-limits + Gemini pricing pages and write BiFrost-style model ids (requires network).
# Optional: INTERSECT=path to JSON or YAML (OpenAI-style data[].id, e.g. catalog-available.snapshot.yaml).
# Override OUT=path for snapshot file (default config/catalog-free-tier.snapshot.yaml).
catalog-free:
	go run ./cmd/catalog-write-free \
		-out "$(if $(OUT),$(OUT),config/catalog-free-tier.snapshot.yaml)" \
		$(if $(INTERSECT),-intersect "$(INTERSECT)",)

# GET BiFrost /v1/models and write YAML (running BiFrost; env BIFROST_BASE_URL, CLAUDIA_UPSTREAM_API_KEY).
# Override OUT=path (default config/catalog-available.snapshot.yaml).
catalog-available:
	go run ./cmd/catalog-write-available \
		-out "$(if $(OUT),$(OUT),config/catalog-available.snapshot.yaml)"

# Runs catalog-available first, then catalog-free: provider-free-tier YAML (groq/gemini ∩ catalog + patterns ollama/*).
# BiFrost must be up for the snapshot step. Network for doc fetches. Optional OUT= for catalog snapshot path (match INTERSECT= if overridden).
# Default PROVIDER_FT_OUT=config/provider-free-tier.generated.yaml (copy/merge into provider-free-tier.yaml if desired).
config-provider-free-tier: catalog-available
	go run ./cmd/catalog-write-free \
		-intersect "$(if $(INTERSECT),$(INTERSECT),config/catalog-available.snapshot.yaml)" \
		-out "$(if $(FREE_OUT),$(FREE_OUT),config/catalog-free-tier.snapshot.yaml)" \
		-provider-free-tier-out "$(if $(PROVIDER_FT_OUT),$(PROVIDER_FT_OUT),config/provider-free-tier.generated.yaml)"

desktop-install:
	$(GITBASH) scripts/desktop-install.sh

desktop-build:
	$(GITBASH) scripts/desktop-build.sh "$(DESKTOP_BIN)"

desktop-run:
	$(GITBASH) scripts/desktop-run.sh "$(DESKTOP_BIN)" "$(MAKE)" desktop -qdrant-bin "$(QDRANT_BIN)" -bifrost-bin "$(BIFROST_BIN)"

run: desktop-run

chimera-run:
	go run $(CHIMERA_CMD_GATEWAY)

claudia-run: chimera-run

# Foreground supervisor: same bin paths as chimera-start --stack (requires make chimera-install).
chimera-serve:
	go run $(CHIMERA_CMD_GATEWAY) serve -qdrant-bin "$(QDRANT_BIN)" -bifrost-bin "$(BIFROST_BIN)"

claudia-serve: chimera-serve

release-install:
	$(GITBASH) scripts/release-install.sh

release-snapshot:
	$(GITBASH) scripts/release-snapshot.sh

# Desktop $(CHIMERA_GATEWAY_BIN) + bifrost-http + qdrant + config → dist/personal/ (needs make install; CGO for desktop build).
package:
	$(GITBASH) scripts/release-package.sh "$(DESKTOP_BIN)"
