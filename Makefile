# Porcelain Makefile — see README.md

CHIMERA_GATEWAY_BIN := chimera-gateway
CHIMERA_INDEX_BIN := chimera-indexer
CHIMERA_SUPERVISOR_BIN := chimera-supervisor
CHIMERA_BROKER_BIN := chimera-broker
CHIMERA_VECTORSTORE_BIN := chimera-vectorstore
CHIMERA_RUNTIME_DIR := chimera
CHIMERA_RUNTIME_BIN_DIR := $(CHIMERA_RUNTIME_DIR)/bin
CHIMERA_RUNTIME_DEPS_DIR := $(CHIMERA_RUNTIME_DIR)/.deps
BIN_STAGE_DIR := bin
CHIMERA_CMD_GATEWAY := ./chimera/chimera-gateway
CHIMERA_CMD_SUPERVISOR := ./chimera/chimera-supervisor
CHIMERA_CMD_BROKER := ./chimera/chimera-broker
CHIMERA_CMD_VECTORSTORE := ./chimera/chimera-vectorstore
CHIMERA_CMD_DESKTOP := ./locus/locus-desktop
CHIMERA_CMD_TOKENCOUNT := ./chimera/cmd/tokencount
CHIMERA_CMD_INDEXER := ./chimera/chimera-indexer
LOCUS_RUNTIME_DIR := locus
LOCUS_RUNTIME_BIN_DIR := $(LOCUS_RUNTIME_DIR)/bin

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
  CHIMERA_GATEWAY_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_GATEWAY_BIN).exe
  CHIMERA_SUPERVISOR_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_SUPERVISOR_BIN).exe
  CHIMERA_BROKER_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_BROKER_BIN).exe
  CHIMERA_VECTORSTORE_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_VECTORSTORE_BIN).exe
  CHIMERA_INDEXER_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_INDEX_BIN).exe
  CHIMERA_GATEWAY_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_GATEWAY_BIN).exe
  CHIMERA_SUPERVISOR_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_SUPERVISOR_BIN).exe
  CHIMERA_BROKER_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_BROKER_BIN).exe
  CHIMERA_VECTORSTORE_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_VECTORSTORE_BIN).exe
  CHIMERA_INDEXER_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_INDEX_BIN).exe
  CHIMERA_BROKER_RUNTIME_BIN := $(CHIMERA_RUNTIME_BIN_DIR)/bifrost-http.exe
  CHIMERA_VECTORSTORE_RUNTIME_BIN := $(CHIMERA_RUNTIME_BIN_DIR)/qdrant.exe
  LOCUS_DESKTOP_BIN := locus-desktop.exe
  LOCUS_DESKTOP_STAGE_OUT := $(BIN_STAGE_DIR)/locus-desktop.exe
else
  ifeq ($(origin GITBASH),undefined)
    GITBASH := bash
  endif
  RACE_GATEWAY := -race
  CHIMERA_GATEWAY_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_GATEWAY_BIN)
  CHIMERA_SUPERVISOR_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_SUPERVISOR_BIN)
  CHIMERA_BROKER_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_BROKER_BIN)
  CHIMERA_VECTORSTORE_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_VECTORSTORE_BIN)
  CHIMERA_INDEXER_BUILD_OUT := $(CHIMERA_RUNTIME_BIN_DIR)/$(CHIMERA_INDEX_BIN)
  CHIMERA_GATEWAY_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_GATEWAY_BIN)
  CHIMERA_SUPERVISOR_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_SUPERVISOR_BIN)
  CHIMERA_BROKER_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_BROKER_BIN)
  CHIMERA_VECTORSTORE_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_VECTORSTORE_BIN)
  CHIMERA_INDEXER_STAGE_OUT := $(BIN_STAGE_DIR)/$(CHIMERA_INDEX_BIN)
  CHIMERA_BROKER_RUNTIME_BIN := $(CHIMERA_RUNTIME_BIN_DIR)/bifrost-http
  CHIMERA_VECTORSTORE_RUNTIME_BIN := $(CHIMERA_RUNTIME_BIN_DIR)/qdrant
  LOCUS_DESKTOP_BIN := locus-desktop
  LOCUS_DESKTOP_STAGE_OUT := $(BIN_STAGE_DIR)/locus-desktop
endif

.PHONY: help bash up configure install chimera-install chimera-test chimera-run clean clean-all clean-data \
	build bin-stage stage-bin-dir chimera-build run \
	chimera-gateway-build chimera-gateway-install chimera-gateway-run chimera-gateway-test chimera-gateway-test-unit chimera-gateway-test-e2e \
	chimera-supervisor-build chimera-supervisor-run chimera-supervisor-test \
	chimera-broker-configure chimera-broker-install chimera-broker-build chimera-broker-run chimera-broker-test chimera-broker-test-unit chimera-broker-test-e2e \
	chimera-vectorstore-configure chimera-vectorstore-install chimera-vectorstore-build chimera-vectorstore-run chimera-vectorstore-test chimera-vectorstore-test-unit chimera-vectorstore-test-e2e \
	chimera-indexer-build chimera-indexer-run chimera-indexer-install chimera-indexer-test \
	locus-install locus-build locus-run locus-test \
	locus-desktop-install locus-desktop-build locus-desktop-run locus-desktop-test \
	tokencount-file catalog-free catalog-available config-provider-free-tier \
	release-install release-snapshot package \
	fmt fmt-check vet test precommit

.DEFAULT_GOAL := help

# One bash process (same as scripts/*.sh) so Win32 Make does not run cmd `echo`/printf per line (quotes + CreateProcess failures).
help:
	@$(GITBASH) scripts/make-help.sh

# --- Full stack onboarding (see docs/plans/makefile-plan.md) ---
up: configure install build locus-desktop-run

install:
	@echo [STEP] Installing all products (Chimera + Locus desktop)
	@$(MAKE) --no-print-directory chimera-install
	@$(MAKE) --no-print-directory locus-desktop-install

build:
	@echo [STEP] Building all products (Chimera + Locus)
	@$(MAKE) --no-print-directory chimera-build
	@$(MAKE) --no-print-directory locus-build

bin-stage: chimera-build locus-build

stage-bin-dir:
	@$(GITBASH) -lc 'mkdir -p "$(BIN_STAGE_DIR)"'

configure:
	$(GITBASH) scripts/configure.sh

run:
	@echo [STEP] Running full stack (Locus)
	@$(MAKE) --no-print-directory build
	@$(MAKE) --no-print-directory locus-run

clean: chimera-clean locus-clean

clean-install: chimera-clean-install locus-clean-install

clean-build: chimera-clean-build locus-clean-build

clean-data: clean-configure
clean-configure: chimera-clean-configure locus-clean-configure

clean-run: chimera-clean-run locus-clean-run

# TODO: remove task and split script
clean-all:
	$(GITBASH) scripts/clean-all-confirm.sh $(CONFIRM)
	$(MAKE) clean
	$(GITBASH) scripts/clean-all.sh

# TODO: create clean tasks for each component (chimera-gateway, chimera-supervisor, chimera-broker, chimera-vectorstore, chimera-indexer, locus-desktop)

chimera-install:
	@echo [STEP] Installing Chimera products (broker, gateway, indexer, vectorstore)
	@$(MAKE) --no-print-directory chimera-broker-install
	@$(MAKE) --no-print-directory chimera-gateway-install
	@$(MAKE) --no-print-directory chimera-indexer-install
	@$(MAKE) --no-print-directory chimera-vectorstore-install

chimera-build:
	@echo [STEP] Building Chimera products (broker, gateway, indexer, supervisor, vectorstore)
	@$(MAKE) --no-print-directory chimera-broker-build
	@$(MAKE) --no-print-directory chimera-gateway-build
	@$(MAKE) --no-print-directory chimera-indexer-build
	@$(MAKE) --no-print-directory chimera-supervisor-build
	@$(MAKE) --no-print-directory chimera-vectorstore-build

chimera-run:
	@echo [STEP] Running Chimera via supervisor
	@$(GITBASH) -lc '"$(CHIMERA_SUPERVISOR_STAGE_OUT)" -broker-bin "$(CHIMERA_BROKER_STAGE_OUT)" -vectorstore-bin "$(CHIMERA_VECTORSTORE_STAGE_OUT)" $(ARGS)'

chimera-test:
	@echo [STEP] Testing Chimera products
	@$(MAKE) --no-print-directory chimera-gateway-test
	@$(MAKE) --no-print-directory chimera-supervisor-test
	@$(MAKE) --no-print-directory chimera-broker-test
	@$(MAKE) --no-print-directory chimera-vectorstore-test
	@$(MAKE) --no-print-directory chimera-indexer-test

# TODO chimera-clean task

# --- Chimera broker ---
chimera-broker-install:
	@echo [STEP] Installing Chimera broker runtime dependency (BiFrost HTTP)
	@$(GITBASH) -lc 'mkdir -p "$(CHIMERA_RUNTIME_DEPS_DIR)"'
	@$(GITBASH) -lc 'CHIMERA_BROKER_BIN_DIR="$(CHIMERA_RUNTIME_BIN_DIR)" DEPS_DIR="$(CHIMERA_RUNTIME_DEPS_DIR)" bash scripts/chimera-broker-install.sh'

chimera-broker-build: chimera-broker-install
	@echo [STEP] Building Chimera broker executable and staging artifacts
	@go build -o $(CHIMERA_BROKER_BUILD_OUT) $(CHIMERA_CMD_BROKER)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_BROKER_BUILD_OUT)" "$(CHIMERA_BROKER_STAGE_OUT)"'
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_BROKER_RUNTIME_BIN)" "$(BIN_STAGE_DIR)/$$(basename "$(CHIMERA_BROKER_RUNTIME_BIN)")"'

chimera-broker-configure:
	@echo [STEP] Generating Chimera broker configuration
	@$(GITBASH) scripts/chimera-broker-configure.sh

chimera-broker-run: chimera-broker-build chimera-broker-configure
	@echo [STEP] Running Chimera broker
	@$(GITBASH) -lc '"$(CHIMERA_BROKER_STAGE_OUT)" -bin "$(CHIMERA_BROKER_RUNTIME_BIN)"'

chimera-broker-test: chimera-broker-test-unit chimera-broker-test-e2e

chimera-broker-test-unit:
	@echo [STEP] Running Chimera broker unit tests
	@go test $(CHIMERA_CMD_BROKER) $(RACE_GATEWAY) -count=1

chimera-broker-test-e2e:
	@echo [STEP] Running Chimera broker end-to-end tests
	@go test $(CHIMERA_CMD_BROKER) $(RACE_GATEWAY) -run E2E -count=1

# TODO: chimera-broker-clean tasks

# --- Chimera gateway ---
chimera-gateway-install:
	@echo [STEP] Installing Chimera gateway to GOBIN
	@go install $(CHIMERA_CMD_GATEWAY)

chimera-gateway-build:
	@echo [STEP] Building Chimera gateway executable and staging artifact
	@$(GITBASH) -lc 'mkdir -p "$(CHIMERA_RUNTIME_BIN_DIR)"'
	@go build -o "$(CHIMERA_GATEWAY_BUILD_OUT)" $(CHIMERA_CMD_GATEWAY)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_GATEWAY_BUILD_OUT)" "$(CHIMERA_GATEWAY_STAGE_OUT)"'

chimera-gateway-run: chimera-gateway-build
	@echo [STEP] Running Chimera gateway on 127.0.0.1:3000
	@$(GITBASH) -lc 'PATH="$$(pwd)/$(CHIMERA_RUNTIME_BIN_DIR):$$(pwd)/$(BIN_STAGE_DIR):$$PATH" "$(CHIMERA_GATEWAY_STAGE_OUT)" -gateway-listen "127.0.0.1:3000" $(ARGS)'

chimera-gateway-test: chimera-gateway-test-unit chimera-gateway-test-e2e

chimera-gateway-test-unit:
	@echo [STEP] Running Chimera gateway unit tests
	@go test $(CHIMERA_CMD_GATEWAY) $(RACE_GATEWAY) -run Test -skip E2E -count=1

chimera-gateway-test-e2e:
	@echo [STEP] Running Chimera gateway end-to-end tests
	@go test $(CHIMERA_CMD_GATEWAY) $(RACE_GATEWAY) -run E2E -count=1

# TODO: chimera-gateway-clean tasks

# --- Chimera indexer ---
chimera-indexer-install:
	@echo [STEP] Installing Chimera indexer to GOBIN
	@go install $(CHIMERA_CMD_INDEXER)

chimera-indexer-build:
	@echo [STEP] Building Chimera indexer executable and staging artifact
	@$(GITBASH) -lc 'mkdir -p "$(CHIMERA_RUNTIME_BIN_DIR)"'
	@go build -o "$(CHIMERA_INDEXER_BUILD_OUT)" $(CHIMERA_CMD_INDEXER)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_INDEXER_BUILD_OUT)" "$(CHIMERA_INDEXER_STAGE_OUT)"'

chimera-indexer-run: chimera-indexer-build
	@echo [STEP] Running Chimera indexer
	@$(GITBASH) -lc '"$(CHIMERA_INDEXER_STAGE_OUT)" $(ARGS)'

chimera-indexer-test: chimera-indexer-test-unit chimera-indexer-test-e2e

chimera-indexer-test-unit:
	@echo [STEP] Running Chimera indexer unit tests
	@go test $(CHIMERA_CMD_INDEXER) $(RACE_GATEWAY) -run TestIndexer -count=1

chimera-indexer-test-e2e:
	@echo [STEP] Running Chimera indexer end-to-end tests
	@go test $(CHIMERA_CMD_INDEXER) $(RACE_GATEWAY) -run E2E_Indexer -count=1

# TODO: add chimera-indexer-test-unit and chimera-indexer-test-e2e
# TODO: chimera-indexer-clean tasks

# --- Chimera supervisor ---
# TODO: add chimera-supervisor-install

chimera-supervisor-build:
	@echo [STEP] Building Chimera supervisor executable and staging artifact
	@$(GITBASH) -lc 'mkdir -p "$(CHIMERA_RUNTIME_BIN_DIR)"'
	@go build -o "$(CHIMERA_SUPERVISOR_BUILD_OUT)" $(CHIMERA_CMD_SUPERVISOR)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_SUPERVISOR_BUILD_OUT)" "$(CHIMERA_SUPERVISOR_STAGE_OUT)"'

chimera-supervisor-run: chimera-supervisor-build
	@echo [STEP] Running Chimera supervisor
	@$(GITBASH) -lc '"$(CHIMERA_SUPERVISOR_STAGE_OUT)" $(ARGS)'

chimera-supervisor-test: chimera-supervisor-test-unit chimera-supervisor-test-e2e

chimera-supervisor-test-unit:
	@echo [STEP] Running Chimera supervisor unit tests
	@go test $(CHIMERA_CMD_SUPERVISOR) $(RACE_GATEWAY) -run TestSupervisor -count=1

chimera-supervisor-test-e2e:
	@echo [STEP] Running Chimera supervisor end-to-end tests
	@go test $(CHIMERA_CMD_SUPERVISOR) $(RACE_GATEWAY) -run E2E_Supervisor -count=1

# TODO: chimera-supervisor-clean tasks

# --- Chimera vectorstore ---
chimera-vectorstore-install:
	@echo [STEP] Installing Chimera vectorstore runtime dependency (Qdrant)
	@$(GITBASH) -lc 'QDRANT_BIN_DIR="$(CHIMERA_RUNTIME_BIN_DIR)" DEPS_DIR="$(CHIMERA_RUNTIME_DEPS_DIR)" bash scripts/chimera-vectorstore-install.sh'

chimera-vectorstore-build: chimera-vectorstore-install
	@echo [STEP] Building Chimera vectorstore executable and staging artifacts
	@go build -o $(CHIMERA_VECTORSTORE_BUILD_OUT) $(CHIMERA_CMD_VECTORSTORE)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_VECTORSTORE_BUILD_OUT)" "$(CHIMERA_VECTORSTORE_STAGE_OUT)"'
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_VECTORSTORE_RUNTIME_BIN)" "$(BIN_STAGE_DIR)/$$(basename "$(CHIMERA_VECTORSTORE_RUNTIME_BIN)")"'

chimera-vectorstore-configure:
	@echo [STEP] Generating Chimera vectorstore configuration
	@$(GITBASH) scripts/chimera-vectorstore-configure.sh

chimera-vectorstore-run: chimera-vectorstore-build chimera-vectorstore-configure
	@echo [STEP] Running Chimera vectorstore
	@$(GITBASH) -lc '"$(CHIMERA_VECTORSTORE_STAGE_OUT)" -bin "$(CHIMERA_VECTORSTORE_RUNTIME_BIN)"'

chimera-vectorstore-test: chimera-vectorstore-test-unit chimera-vectorstore-test-e2e

chimera-vectorstore-test-unit:
	@echo [STEP] Running Chimera vectorstore unit tests
	@go test $(CHIMERA_CMD_VECTORSTORE) $(RACE_GATEWAY) -count=1

chimera-vectorstore-test-e2e:
	@echo [STEP] Running Chimera vectorstore end-to-end tests
	@go test $(CHIMERA_CMD_VECTORSTORE) $(RACE_GATEWAY) -run E2E -count=1

# TODO: chimera-vectorstore-clean tasks

# --- Locus ---
locus-install:
	@echo [STEP] Installing Locus products
	@$(MAKE) --no-print-directory locus-desktop-install

locus-build:
	@echo [STEP] Building Locus products
	@$(MAKE) --no-print-directory locus-desktop-build

locus-run:
	@echo [STEP] Running Locus products
	@$(MAKE) --no-print-directory locus-desktop-run

locus-test:
	@echo [STEP] Testing Locus products
	@$(MAKE) --no-print-directory locus-desktop-test

locus-clean: locus-desktop-clean

locus-clean-install: locus-desktop-clean-install

locus-clean-build: locus-desktop-clean-build

locus-clean-configure: locus-desktop-clean-configure

locus-clean-run: locus-desktop-clean-run

# --- Locus desktop ---
locus-desktop-install:
	@echo [STEP] Installing Locus desktop prerequisites
	@$(GITBASH) scripts/locus-desktop-install.sh

locus-desktop-build:
	@echo [STEP] Building Locus desktop executable and staging artifact
	@$(GITBASH) -lc 'mkdir -p "$(LOCUS_RUNTIME_BIN_DIR)"'
	@$(GITBASH) scripts/locus-desktop-build.sh "$(LOCUS_DESKTOP_BIN)"
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(LOCUS_RUNTIME_BIN_DIR)/$(LOCUS_DESKTOP_BIN)" "$(LOCUS_DESKTOP_STAGE_OUT)"'

locus-desktop-run:
	@echo [STEP] Running Locus desktop (with Chimera runtime dependencies)
	@$(GITBASH) -lc '"$(LOCUS_DESKTOP_STAGE_OUT)" desktop \
		-broker-bin "$(CHIMERA_BROKER_STAGE_OUT)" \
		-vectorstore-bin "$(CHIMERA_VECTORSTORE_STAGE_OUT)" \
		$(ARGS)'

locus-desktop-test: export CGO_ENABLED := 1
locus-desktop-test:
	@echo [STEP] Running Locus desktop tests (desktop/CGO)
	@go test -tags desktop $(CHIMERA_CMD_DESKTOP) $(RACE_GATEWAY) -count=1

# TODO: locus-desktop-clean tasks

# --- Tools ---
bash:
	$(GITBASH) -il

tokencount-file:
ifeq ($(FILE),)
	$(error FILE is required, e.g. make tokencount-file FILE=temp/groq-request.json)
endif
	go run $(CHIMERA_CMD_TOKENCOUNT) -f "$(FILE)"

# Fetch Groq rate-limits + Gemini pricing pages and write BiFrost-style model ids (requires network).
# Optional: INTERSECT=path to JSON or YAML (OpenAI-style data[].id, e.g. catalog-available.snapshot.yaml).
# Override OUT=path for snapshot file (default config/catalog-free-tier.snapshot.yaml).
catalog-fetch-free:
	go run ./chimera/cmd/catalog-write-free \
		-out "$(if $(OUT),$(OUT),config/catalog-free-tier.snapshot.yaml)" \
		$(if $(INTERSECT),-intersect "$(INTERSECT)",)

# GET BiFrost /v1/models and write YAML (running BiFrost; env BIFROST_BASE_URL, Chimera_UPSTREAM_API_KEY).
# Override OUT=path (default config/catalog-available.snapshot.yaml).
catalog-fetch-available:
	go run ./chimera/cmd/catalog-write-available \
		-out "$(if $(OUT),$(OUT),config/catalog-available.snapshot.yaml)"

# Runs catalog-available first, then catalog-free: provider-free-tier YAML (groq/gemini ∩ catalog + patterns ollama/*).
# BiFrost must be up for the snapshot step. Network for doc fetches. Optional OUT= for catalog snapshot path (match INTERSECT= if overridden).
# Default PROVIDER_FT_OUT=config/provider-free-tier.generated.yaml (copy/merge into provider-free-tier.yaml if desired).
catalog-calculate: catalog-available
	go run ./chimera/cmd/catalog-write-free \
		-intersect "$(if $(INTERSECT),$(INTERSECT),config/catalog-available.snapshot.yaml)" \
		-out "$(if $(FREE_OUT),$(FREE_OUT),config/catalog-free-tier.snapshot.yaml)" \
		-provider-free-tier-out "$(if $(PROVIDER_FT_OUT),$(PROVIDER_FT_OUT),config/provider-free-tier.generated.yaml)"


# --- Release packaging ---

# TODO: REVISIT THIS TASK
# release-install:
# 	$(GITBASH) scripts/release-install.sh
# TODO: REVISIT THIS TASK
# release-snapshot:
# 	$(GITBASH) scripts/release-snapshot.sh
# TODO: REVISIT THIS TASK
# package:
# 	$(GITBASH) scripts/package.sh "$(LOCUS_DESKTOP_BIN)"

# --- Quality gates ---
fmt:
	gofmt -w cmd internal

fmt-check:
	$(GITBASH) scripts/fmt-check.sh

vet:
	go vet ./...

precommit: fmt-check vet test
