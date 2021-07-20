# ##
#
# Run 'make help' for a summary
#
# ##

# Binaries
BIN         := func
BIN_DARWIN  ?= $(BIN)_darwin_amd64
BIN_LINUX   ?= $(BIN)_linux_amd64
BIN_WINDOWS ?= $(BIN)_windows_amd64.exe

# Version
# A verbose version is built into the binary including a date stamp, git commit
# hash and the version tag of the current commit (semver) if it exists.
# If the current commit does not have a semver tag, 'tip' is used.
DATE    := $(shell date -u +"%Y%m%dT%H%M%SZ")
HASH    := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG    := $(shell git tag --points-at HEAD)
VERS    ?= $(shell [ -z $(VTAG) ] && echo 'tip' || echo $(VTAG) )
LDFLAGS := "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)"

# Templates
# Built into the binary are the contents of ./templates.  This is done by
# running 'pkger' which generates pkged.go, containing a go-encoded version of
# the templates directory.
PKGER           ?= pkger

# Code is all go source files, used for build target freshness checks
CODE := $(shell find . -name '*.go')

all: build
	# Run 'make help' for make target documentation.

# Print Help Text
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

###############
# Development #
###############

##@ Development

build: $(CODE) ## (default) Build binary for current OS
	env CGO_ENABLED=0 go build -ldflags $(LDFLAGS) ./cmd/$(BIN)

test: $(CODE) ## Run core unit tests
	go test -race -cover -coverprofile=coverage.out ./...

check: bin/golangci-lint ## Check code quality (lint)
	./bin/golangci-lint run --timeout 300s
	cd test/_e2e && ../../bin/golangci-lint run --timeout 300s

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.40.1

clean: ## Remove generated artifacts such as binaries
	rm -f $(BIN) $(BIN_WINDOWS) $(BIN_LINUX) $(BIN_DARWIN)
	-rm -f coverage.out

clean-templates: 
	# Clearing caches in ./templates
	@rm -rf templates/node/events/node_modules
	@rm -rf templates/node/http/node_modules
	@rm -rf templates/python/events/__pycache__
	@rm -rf templates/python/http/__pycache__
	@rm -rf templates/typescript/events/node_modules
	@rm -rf templates/typescript/http/node_modules
	@rm -rf templates/rust/events/target
	@rm -rf templates/rust/http/target

################
# Templates #
################

##@ Builtin Language Packs

templates: test-templates pkged.go ## Run template unit tests and update pkged.go

pkged.go: clean-templates
	# Encoding ./templates as pkged.go (requires 'pkger':  go get github.com/markbates/pkger/cmd/pkger)
	$(PKGER)

test-templates: test-go test-node test-python test-quarkus test-rust test-typescript ## Run all template tests

test-go: ## Test Go templates
	cd templates/go/events && go test
	cd templates/go/http && go test

test-node: ## Test Node templates
	cd templates/node/events && npm ci && npm test && rm -rf node_modules
	cd templates/node/http && npm ci && npm test && rm -rf node_modules

test-python: ## Test Python templates
	cd templates/python/events && pip3 install -r requirements.txt && python3 test_func.py
	cd templates/python/http && python3 test_func.py

test-quarkus: ## Test Quarkus templates
	cd templates/quarkus/events && mvn test && mvn clean
	cd templates/quarkus/http && mvn test && mvn clean

test-rust: ## Test Rust templates
	cd templates/rust/events && cargo test && cargo clean
	cd templates/rust/http && cargo test && cargo clean

test-typescript: ## Test Typescript templates
	cd templates/typescript/events && npm ci && npm test && rm -rf node_modules build
	cd templates/typescript/http && npm ci && npm test && rm -rf node_modules build


###################
# Release Testing #
###################

##@ Extended Testing (cluster required)

test-integration: ## Run integration tests using an available cluster.
	go test -tags integration ./... -v

test-e2e: ## Run end-to-end tests using an available cluster.
	./test/run_e2e_test.sh


######################
# Release Artifacts  #
######################

##@ Release Artifacts

cross-platform: darwin linux windows ## Build all distributable (cross-platform) binaries

darwin: $(BIN_DARWIN) ## Build for Darwin (macOS)

$(BIN_DARWIN): pkged.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DARWIN) -ldflags $(LDFLAGS) ./cmd/$(BIN)

linux: $(BIN_LINUX) ## Build for Linux

$(BIN_LINUX): pkged.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_LINUX) -ldflags $(LDFLAGS) ./cmd/$(BIN)

windows: $(BIN_WINDOWS) ## Build for Windows

$(BIN_WINDOWS): pkged.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_WINDOWS) -ldflags $(LDFLAGS) ./cmd/$(BIN)

