# Porcelain Makefile — see README.md

CHIMERA_GATEWAY_BIN := chimera-gateway
CHIMERA_INDEX_BIN := chimera-indexer
CHIMERA_SUPERVISOR_BIN := chimera-supervisor
CHIMERA_BROKER_BIN := chimera-broker
CHIMERA_VECTORSTORE_BIN := chimera-vectorstore
CHIMERA_RUNTIME_DIR := chimera
CHIMERA_RUNTIME_BIN_DIR := $(CHIMERA_RUNTIME_DIR)/bin
CHIMERA_RUNTIME_DEPS_DIR := $(CHIMERA_RUNTIME_DIR)/.deps
CHIMERA_CMD_GATEWAY := ./chimera/chimera-gateway
CHIMERA_ADMINUI_EMBED_DIR := chimera/chimera-gateway/internal/server/adminui/embed
CHIMERA_CMD_SUPERVISOR := ./chimera/chimera-supervisor
CHIMERA_CMD_BROKER := ./chimera/chimera-broker
CHIMERA_CMD_VECTORSTORE := ./chimera/chimera-vectorstore
LOCUS_CMD_DESKTOP := ./locus/locus-desktop
CHIMERA_CMD_TOKENCOUNT := ./chimera/cmd/tokencount
CHIMERA_CMD_INDEXER := ./chimera/chimera-indexer

LOCUS_RUNTIME_DIR := locus
LOCUS_RUNTIME_BIN_DIR := $(LOCUS_RUNTIME_DIR)/bin

BIN_STAGE_DIR := bin
FMT_DIRS := chimera locus internal


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

# Linux/macOS: use bash for recipes; unquoted () in @echo are subshell syntax in sh/bash.
# Windows cmd echo prints literal quote characters — route parenthetical lines through GITBASH.
ifneq ($(OS),Windows_NT)
SHELL := /bin/bash
endif

define step_msg
	@$(GITBASH) -c "echo '[STEP] $(1)'"
endef

define skip_desktop_msg
	@$(GITBASH) -c "echo '[SKIP] $(1) (SKIP_DESKTOP=1)'"
endef

.PHONY: help bash up configure install chimera-install chimera-test chimera-run \
	clean clean-install clean-build clean-configure clean-data clean-run clean-all \
	chimera-clean chimera-clean-install chimera-clean-build chimera-clean-configure chimera-clean-run \
	chimera-gateway-clean chimera-gateway-clean-install chimera-gateway-clean-build chimera-gateway-clean-configure chimera-gateway-clean-run \
	chimera-supervisor-clean chimera-supervisor-clean-install chimera-supervisor-clean-build chimera-supervisor-clean-configure chimera-supervisor-clean-run \
	chimera-broker-clean chimera-broker-clean-install chimera-broker-clean-build chimera-broker-clean-run \
	chimera-vectorstore-clean chimera-vectorstore-clean-install chimera-vectorstore-clean-build chimera-vectorstore-clean-run \
	chimera-indexer-clean chimera-indexer-clean-install chimera-indexer-clean-build chimera-indexer-clean-configure chimera-indexer-clean-run \
	locus-clean locus-clean-install locus-clean-build locus-clean-configure locus-clean-run \
	locus-desktop-clean locus-desktop-clean-install locus-desktop-clean-build locus-desktop-clean-configure locus-desktop-clean-run \
	build bin-stage stage-bin-dir chimera-build run \
	chimera-gateway-build chimera-gateway-install chimera-gateway-run component-gallery-check chimera-gateway-test chimera-gateway-test-unit chimera-gateway-test-e2e \
	chimera-supervisor-build chimera-supervisor-run chimera-supervisor-test chimera-supervisor-test-unit chimera-supervisor-test-e2e \
	chimera-broker-install chimera-broker-build chimera-broker-run chimera-broker-test chimera-broker-test-unit chimera-broker-test-e2e \
	chimera-vectorstore-install chimera-vectorstore-build chimera-vectorstore-run chimera-vectorstore-test chimera-vectorstore-test-unit chimera-vectorstore-test-e2e \
	chimera-indexer-build chimera-indexer-run chimera-indexer-install chimera-indexer-configure chimera-indexer-test chimera-indexer-test-unit chimera-indexer-test-e2e \
	locus-install locus-build locus-run locus-test locus-vet-if-enabled locus-test-if-enabled \
	locus-test-unit-if-enabled locus-test-e2e-if-enabled \
	locus-desktop-install locus-desktop-build locus-desktop-run locus-desktop-dev-ui chimera-supervisor-dev-ui \
	locus-desktop-test locus-desktop-test-unit locus-desktop-test-e2e \
	tokencount-file catalog-free catalog-available config-provider-free-tier \
	release-install release-build release-package \
	fmt fmt-check vet vet-desktop test precommit operator-contracts-generate operator-contracts-check

.DEFAULT_GOAL := help

# One bash process (same as scripts/*.sh) so Win32 Make does not run cmd `echo`/printf per line (quotes + CreateProcess failures).
help:
	@$(GITBASH) scripts/make-help.sh

# --- Full stack onboarding (see docs/plans/makefile-plan.md) ---
up: configure install build locus-desktop-run

install:
	$(call step_msg,Installing all products (Chimera + Locus desktop))
	@$(MAKE) --no-print-directory chimera-install
	@$(MAKE) --no-print-directory locus-desktop-install

build:
	$(call step_msg,Building all products (Chimera + Locus))
	@$(MAKE) --no-print-directory chimera-build
	@$(MAKE) --no-print-directory locus-build

bin-stage: chimera-build locus-build

stage-bin-dir:
	@$(GITBASH) -lc 'mkdir -p "$(BIN_STAGE_DIR)"'

configure:
	$(MAKE) --no-print-directory chimera-configure

run:
	$(call step_msg,Running full stack (Locus))
	@$(MAKE) --no-print-directory build
	@$(MAKE) --no-print-directory locus-run

test: chimera-test locus-test-if-enabled
test-unit: chimera-test-unit locus-test-unit-if-enabled
test-e2e: chimera-test-e2e locus-test-e2e-if-enabled

vet:
	$(MAKE) --no-print-directory chimera-vet locus-vet-if-enabled

vet-desktop:
	$(MAKE) --no-print-directory chimera-vet locus-vet-desktop

locus-vet-if-enabled:
ifeq ($(SKIP_DESKTOP),1)
	$(call skip_desktop_msg,locus-vet)
else
	@$(MAKE) --no-print-directory locus-vet
endif

locus-test-if-enabled:
ifeq ($(SKIP_DESKTOP),1)
	$(call skip_desktop_msg,locus-test)
else
	@$(MAKE) --no-print-directory locus-test
endif

locus-test-unit-if-enabled:
ifeq ($(SKIP_DESKTOP),1)
	$(call skip_desktop_msg,locus-test-unit)
else
	@$(MAKE) --no-print-directory locus-test-unit
endif

locus-test-e2e-if-enabled:
ifeq ($(SKIP_DESKTOP),1)
	$(call skip_desktop_msg,locus-test-e2e)
else
	@$(MAKE) --no-print-directory locus-test-e2e
endif

clean:
	$(GITBASH) scripts/clean.sh

clean-install: chimera-clean-install locus-clean-install

clean-build: chimera-clean-build locus-clean-build

clean-configure: chimera-clean-configure locus-clean-configure

clean-data:
	@$(MAKE) --no-print-directory chimera-clean-run CONFIRM=$(CONFIRM)

clean-run: chimera-clean-run locus-clean-run

clean-all:
	$(GITBASH) scripts/clean-all.sh $(CONFIRM)

chimera-install:
	$(call step_msg,Installing Chimera products (broker, gateway, indexer, vectorstore))
	@$(MAKE) --no-print-directory chimera-broker-install
	@$(MAKE) --no-print-directory chimera-gateway-install
	@$(MAKE) --no-print-directory chimera-indexer-install
	@$(MAKE) --no-print-directory chimera-vectorstore-install

chimera-build:
	$(call step_msg,Building Chimera products (broker, gateway, indexer, supervisor, vectorstore))
	@$(MAKE) --no-print-directory chimera-broker-build
	@$(MAKE) --no-print-directory chimera-gateway-build
	@$(MAKE) --no-print-directory chimera-indexer-build
	@$(MAKE) --no-print-directory chimera-supervisor-build
	@$(MAKE) --no-print-directory chimera-vectorstore-build

chimera-configure:
	@echo [STEP] Generating Chimera configuration
	@$(GITBASH) scripts/chimera-configure.sh

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

chimera-test-unit:
	@echo [STEP] Running Chimera unit tests
	@$(MAKE) --no-print-directory chimera-gateway-test-unit
	@$(MAKE) --no-print-directory chimera-supervisor-test-unit
	@$(MAKE) --no-print-directory chimera-broker-test-unit
	@$(MAKE) --no-print-directory chimera-vectorstore-test-unit
	@$(MAKE) --no-print-directory chimera-indexer-test-unit

chimera-test-e2e:
	@echo [STEP] Running Chimera end-to-end tests
	@$(MAKE) --no-print-directory chimera-gateway-test-e2e
	@$(MAKE) --no-print-directory chimera-supervisor-test-e2e
	@$(MAKE) --no-print-directory chimera-broker-test-e2e
	@$(MAKE) --no-print-directory chimera-vectorstore-test-e2e
	@$(MAKE) --no-print-directory chimera-indexer-test-e2e

chimera-vet:
	go vet ./chimera/...

chimera-clean: chimera-gateway-clean chimera-supervisor-clean chimera-broker-clean chimera-vectorstore-clean chimera-indexer-clean
	@$(GITBASH) -lc 'rm -rf dist'

chimera-clean-install: chimera-gateway-clean-install chimera-supervisor-clean-install chimera-broker-clean-install chimera-vectorstore-clean-install chimera-indexer-clean-install

chimera-clean-build: chimera-gateway-clean-build chimera-supervisor-clean-build chimera-broker-clean-build chimera-vectorstore-clean-build chimera-indexer-clean-build

chimera-clean-configure: chimera-gateway-clean-configure chimera-indexer-clean-configure

chimera-clean-run: chimera-gateway-clean-run chimera-supervisor-clean-run chimera-broker-clean-run chimera-vectorstore-clean-run chimera-indexer-clean-run

# --- Chimera broker ---
chimera-broker-install:
	$(call step_msg,Installing Chimera broker runtime dependency (BiFrost HTTP))
	@$(GITBASH) -lc 'mkdir -p "$(CHIMERA_RUNTIME_DEPS_DIR)"'
	@$(GITBASH) -lc 'CHIMERA_BROKER_BIN_DIR="$(CHIMERA_RUNTIME_BIN_DIR)" DEPS_DIR="$(CHIMERA_RUNTIME_DEPS_DIR)" BIFROST_SKIP_UI="$(BIFROST_SKIP_UI)" bash scripts/chimera-broker-install.sh'

chimera-broker-build: chimera-broker-install
	@echo [STEP] Building Chimera broker executable and staging artifacts
	@go build -o $(CHIMERA_BROKER_BUILD_OUT) $(CHIMERA_CMD_BROKER)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_BROKER_BUILD_OUT)" "$(CHIMERA_BROKER_STAGE_OUT)"'
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_BROKER_RUNTIME_BIN)" "$(BIN_STAGE_DIR)/$$(basename "$(CHIMERA_BROKER_RUNTIME_BIN)")"'

chimera-broker-run:
	@echo [STEP] Running Chimera broker
	@$(GITBASH) -lc '"$(CHIMERA_BROKER_STAGE_OUT)" -bin "$(CHIMERA_BROKER_RUNTIME_BIN)"'

chimera-broker-test: chimera-broker-test-unit chimera-broker-test-e2e

chimera-broker-test-unit:
	@echo [STEP] Running Chimera broker unit tests
	@go test $(CHIMERA_CMD_BROKER)/... $(RACE_GATEWAY) -run Test -skip E2E -count=1

chimera-broker-test-e2e:
	@echo [STEP] Running Chimera broker end-to-end tests
	@go test $(CHIMERA_CMD_BROKER) $(RACE_GATEWAY) -run E2E -count=1

chimera-broker-clean: chimera-broker-clean-build chimera-broker-clean-install chimera-broker-clean-run

chimera-broker-clean-install:
	$(GITBASH) scripts/clean-product.sh broker install

chimera-broker-clean-build:
	$(GITBASH) scripts/clean-product.sh broker build

chimera-broker-clean-run:
	$(GITBASH) scripts/clean-product.sh broker run $(CONFIRM)

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

chimera-gateway-run: chimera-configure
	@echo [STEP] Running Chimera gateway on 127.0.0.1:3000
	@$(GITBASH) -lc 'PATH="$$(pwd)/$(CHIMERA_RUNTIME_BIN_DIR):$$(pwd)/$(BIN_STAGE_DIR):$$PATH" "$(CHIMERA_GATEWAY_STAGE_OUT)" -gateway-listen "127.0.0.1:3000" $(ARGS)'

component-gallery-check:
	@echo [STEP] Checking component gallery asset paths
	@$(GITBASH) scripts/check-component-gallery-paths.sh

chimera-gateway-test: component-gallery-check operator-contracts-check contracts-check chimera-gateway-test-unit chimera-gateway-test-e2e

chimera-gateway-test-unit:
	@echo [STEP] Running Chimera gateway unit tests
	@go test $(CHIMERA_CMD_GATEWAY)/... $(RACE_GATEWAY) -run Test -skip E2E -count=1

chimera-gateway-test-e2e:
	@echo [STEP] Running Chimera gateway end-to-end tests
	@go test $(CHIMERA_CMD_GATEWAY) $(RACE_GATEWAY) -run E2E -count=1

chimera-gateway-clean: chimera-gateway-clean-build chimera-gateway-clean-install chimera-gateway-clean-configure chimera-gateway-clean-run

chimera-gateway-clean-install:
	$(GITBASH) scripts/clean-product.sh gateway install

chimera-gateway-clean-build:
	$(GITBASH) scripts/clean-product.sh gateway build

chimera-gateway-clean-configure:
	$(GITBASH) scripts/clean-product.sh gateway configure

chimera-gateway-clean-run:
	$(GITBASH) scripts/clean-product.sh gateway run $(CONFIRM)

# --- Chimera indexer ---
chimera-indexer-configure:
	@echo [STEP] Generating Chimera indexer configuration
	@$(GITBASH) scripts/configure-copy.sh indexer.example.yaml indexer.yaml

chimera-indexer-install:
	@echo [STEP] Installing Chimera indexer to GOBIN
	@go install $(CHIMERA_CMD_INDEXER)

chimera-indexer-build:
	@echo [STEP] Building Chimera indexer executable and staging artifact
	@$(GITBASH) -lc 'mkdir -p "$(CHIMERA_RUNTIME_BIN_DIR)"'
	@go build -o "$(CHIMERA_INDEXER_BUILD_OUT)" $(CHIMERA_CMD_INDEXER)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_INDEXER_BUILD_OUT)" "$(CHIMERA_INDEXER_STAGE_OUT)"'

chimera-indexer-run:
	@echo [STEP] Running Chimera indexer
	@$(GITBASH) -lc '"$(CHIMERA_INDEXER_STAGE_OUT)" $(ARGS)'

chimera-indexer-test: chimera-indexer-test-unit chimera-indexer-test-e2e

chimera-indexer-test-unit:
	@echo [STEP] Running Chimera indexer unit tests
	@go test $(CHIMERA_CMD_INDEXER)/... $(RACE_GATEWAY) -run Test -skip E2E -count=1

chimera-indexer-test-e2e:
	@echo [STEP] Running Chimera indexer end-to-end tests
	@go test $(CHIMERA_CMD_INDEXER) $(RACE_GATEWAY) -run E2E -count=1

chimera-indexer-clean: chimera-indexer-clean-build chimera-indexer-clean-install chimera-indexer-clean-configure chimera-indexer-clean-run

chimera-indexer-clean-install:
	$(GITBASH) scripts/clean-product.sh indexer install

chimera-indexer-clean-build:
	$(GITBASH) scripts/clean-product.sh indexer build

chimera-indexer-clean-configure:
	$(GITBASH) scripts/clean-product.sh indexer configure

chimera-indexer-clean-run:
	$(GITBASH) scripts/clean-product.sh indexer run $(CONFIRM)

# --- Chimera supervisor ---
# TODO: add chimera-supervisor-install

chimera-supervisor-build:
	@echo [STEP] Building Chimera supervisor executable and staging artifact
	@$(GITBASH) -lc 'mkdir -p "$(CHIMERA_RUNTIME_BIN_DIR)"'
	@go build -o "$(CHIMERA_SUPERVISOR_BUILD_OUT)" $(CHIMERA_CMD_SUPERVISOR)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_SUPERVISOR_BUILD_OUT)" "$(CHIMERA_SUPERVISOR_STAGE_OUT)"'

chimera-supervisor-run: chimera-configure
	@echo [STEP] Running Chimera supervisor
	@$(GITBASH) -lc '"$(CHIMERA_SUPERVISOR_STAGE_OUT)" $(ARGS)'

chimera-supervisor-test: chimera-supervisor-test-unit chimera-supervisor-test-e2e

chimera-supervisor-test-unit:
	@echo [STEP] Running Chimera supervisor unit tests
	@go test $(CHIMERA_CMD_SUPERVISOR)/... $(RACE_GATEWAY) -run Test -skip E2E -count=1

chimera-supervisor-test-e2e:
	@echo [STEP] Running Chimera supervisor end-to-end tests
	@go test $(CHIMERA_CMD_SUPERVISOR) $(RACE_GATEWAY) -run E2E -count=1

chimera-supervisor-clean: chimera-supervisor-clean-build chimera-supervisor-clean-install chimera-supervisor-clean-configure chimera-supervisor-clean-run

chimera-supervisor-clean-install:
	$(GITBASH) scripts/clean-product.sh supervisor install

chimera-supervisor-clean-build:
	$(GITBASH) scripts/clean-product.sh supervisor build

chimera-supervisor-clean-configure:
	$(GITBASH) scripts/clean-product.sh supervisor configure

chimera-supervisor-clean-run:
	$(GITBASH) scripts/clean-product.sh supervisor run $(CONFIRM)

# --- Chimera vectorstore ---
chimera-vectorstore-install:
	$(call step_msg,Installing Chimera vectorstore runtime dependency (Qdrant))
	@$(GITBASH) -lc 'QDRANT_BIN_DIR="$(CHIMERA_RUNTIME_BIN_DIR)" DEPS_DIR="$(CHIMERA_RUNTIME_DEPS_DIR)" bash scripts/chimera-vectorstore-install.sh'

chimera-vectorstore-build: chimera-vectorstore-install
	@echo [STEP] Building Chimera vectorstore executable and staging artifacts
	@go build -o $(CHIMERA_VECTORSTORE_BUILD_OUT) $(CHIMERA_CMD_VECTORSTORE)
	@$(MAKE) stage-bin-dir
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_VECTORSTORE_BUILD_OUT)" "$(CHIMERA_VECTORSTORE_STAGE_OUT)"'
	@$(GITBASH) -lc 'cp -f "$(CHIMERA_VECTORSTORE_RUNTIME_BIN)" "$(BIN_STAGE_DIR)/$$(basename "$(CHIMERA_VECTORSTORE_RUNTIME_BIN)")"'

chimera-vectorstore-run:
	@echo [STEP] Running Chimera vectorstore
	@$(GITBASH) -lc '"$(CHIMERA_VECTORSTORE_STAGE_OUT)" -bin "$(CHIMERA_VECTORSTORE_RUNTIME_BIN)"'

chimera-vectorstore-test: chimera-vectorstore-test-unit chimera-vectorstore-test-e2e

chimera-vectorstore-test-unit:
	@echo [STEP] Running Chimera vectorstore unit tests
	@go test $(CHIMERA_CMD_VECTORSTORE)/... $(RACE_GATEWAY) -run Test -skip E2E -count=1

chimera-vectorstore-test-e2e:
	@echo [STEP] Running Chimera vectorstore end-to-end tests
	@go test $(CHIMERA_CMD_VECTORSTORE) $(RACE_GATEWAY) -run E2E -count=1

chimera-vectorstore-clean: chimera-vectorstore-clean-build chimera-vectorstore-clean-install chimera-vectorstore-clean-run

chimera-vectorstore-clean-install:
	$(GITBASH) scripts/clean-product.sh vectorstore install

chimera-vectorstore-clean-build:
	$(GITBASH) scripts/clean-product.sh vectorstore build

chimera-vectorstore-clean-run:
	$(GITBASH) scripts/clean-product.sh vectorstore run $(CONFIRM)

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

locus-vet:
	go vet ./locus/...

locus-vet-desktop:
	go vet $(LOCUS_CMD_DESKTOP)/...

locus-test-unit:
	@$(MAKE) --no-print-directory locus-desktop-test-unit

locus-test-e2e:
	@$(MAKE) --no-print-directory locus-desktop-test-e2e

locus-clean: locus-desktop-clean

locus-clean-install: locus-desktop-clean-install

locus-clean-build: locus-desktop-clean-build

locus-clean-configure: locus-desktop-clean-configure

locus-clean-run: locus-desktop-clean-run

locus-desktop-clean: locus-desktop-clean-build locus-desktop-clean-install locus-desktop-clean-configure locus-desktop-clean-run

locus-desktop-clean-install:
	$(GITBASH) scripts/clean-product.sh desktop install

locus-desktop-clean-build:
	$(GITBASH) scripts/clean-product.sh desktop build

locus-desktop-clean-configure:
	$(GITBASH) scripts/clean-product.sh desktop configure

locus-desktop-clean-run:
	$(GITBASH) scripts/clean-product.sh desktop run

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
	$(call step_msg,Running Locus desktop (with Chimera runtime dependencies))
	@$(GITBASH) -lc '"$(LOCUS_DESKTOP_STAGE_OUT)" desktop \
		-broker-bin "$(CHIMERA_BROKER_STAGE_OUT)" \
		-vectorstore-bin "$(CHIMERA_VECTORSTORE_STAGE_OUT)" \
		$(ARGS)'

# Serves operator UI assets from the repo embed tree (CHIMERA_ADMINUI_ROOT); loopback listen only.
locus-desktop-dev-ui:
	@echo [STEP] Running Locus desktop with filesystem operator UI assets
	@$(GITBASH) -lc 'export CHIMERA_ADMINUI_ROOT="$$(pwd)/$(CHIMERA_ADMINUI_EMBED_DIR)" && $(MAKE) --no-print-directory locus-desktop-run'

chimera-supervisor-dev-ui: chimera-configure
	@echo [STEP] Running chimera-supervisor with filesystem operator UI assets
	@$(GITBASH) -lc 'export CHIMERA_ADMINUI_ROOT="$$(pwd)/$(CHIMERA_ADMINUI_EMBED_DIR)" && $(MAKE) --no-print-directory chimera-supervisor-run'

locus-desktop-test: locus-desktop-test-unit locus-desktop-test-e2e

locus-desktop-test-unit:
	$(call step_msg,Running Locus desktop unit tests (desktop/CGO))
	@go test $(LOCUS_CMD_DESKTOP)/... $(RACE_GATEWAY) -run Test -skip E2E -count=1

locus-desktop-test-e2e:
	$(call step_msg,Running Locus desktop end-to-end tests (desktop/CGO))
	@go test $(LOCUS_CMD_DESKTOP)/internal/... $(RACE_GATEWAY) -run E2E -count=1

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

# --- Release (install → build → package) ---

release-install:
	$(call step_msg,Installing release tooling (GoReleaser))
	@$(GITBASH) scripts/release-install.sh

release-build: release-install
	$(call step_msg,Building release archives (GoReleaser snapshot))
	@$(GITBASH) scripts/release-build.sh

release-package: chimera-build locus-desktop-build
	@echo [STEP] Packaging personal desktop bundle
	@$(GITBASH) scripts/release-package.sh "$(LOCUS_DESKTOP_BIN)"

# --- Operator UI contracts (Phase 3: internal/naming → embedui/logs/contracts.js) ---

contracts-generate:
	@echo [STEP] Generating data contracts from internal/naming
	@go generate ./internal/naming/...
	$(call step_msg,Regenerating operator copy (messages.yaml bootstrap + operator_copy.js))
	@go run ./internal/operatorcopy/cmd/bootstrap
	@go generate ./internal/operatorcopy/...

contracts-check:
	@echo [STEP] Checking data contracts are up to date
	@go test ./internal/naming/... -run TestGeneratedContractsJSMatchesFile -count=1
	@echo [STEP] Checking operator_copy.js and log_messages.go are up to date
	@go test ./internal/operatorcopy/... -run TestGeneratedOperatorCopyJSMatchesFile -count=1
	@go test ./internal/naming/... -run 'TestGeneratedLogMessagesGoMatchesFile|TestLogMessageConstsHaveRegistryEntry' -count=1
	@$(GITBASH) scripts/operatorcopy-msg-audit.sh

# Supervisor log normalization fidelity (docs/plans/log-supervisor-normalization-fidelity.md).
test-log-fidelity:
	@$(GITBASH) scripts/test-log-fidelity.sh

# --- Quality gates ---

fmt:
	gofmt -w $(FMT_DIRS)

fmt-check:
	$(GITBASH) scripts/fmt-check.sh $(FMT_DIRS)

precommit: fmt-check vet test
