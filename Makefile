# ##
#
# Run 'make help' for a summary
#
# ##

# Binaries
BIN         := func
BIN_DARWIN_AMD64   ?= $(BIN)_darwin_amd64
BIN_DARWIN_ARM64   ?= $(BIN)_darwin_arm64
BIN_LINUX_AMD64   ?= $(BIN)_linux_amd64
BIN_LINUX_ARM64   ?= $(BIN)_linux_arm64
BIN_LINUX_PPC64LE ?= $(BIN)_linux_ppc64le
BIN_LINUX_S390X   ?= $(BIN)_linux_s390x
BIN_WINDOWS ?= $(BIN)_windows_amd64.exe

# Version
# A verbose version is built into the binary including a date stamp, git commit
# hash and the version tag of the current commit (semver) if it exists.
# If the current commit does not have a semver tag, 'tip' is used, unless there
# is a TAG environment variable. Precedence is git tag, environment variable, 'tip'
DATE    := $(shell date -u +"%Y%m%dT%H%M%SZ")
HASH    := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG    := $(shell git tag --points-at HEAD | head -1)
VTAG    := $(shell [ -z $(VTAG) ] && echo $(ETAG) || echo $(VTAG))
VERS    ?= $(shell [ -z $(VTAG) ] && echo 'tip' || echo $(VTAG) )
LDFLAGS := "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)"

# All Code prerequisites, including generated files, etc.
CODE := $(shell find . -name '*.go') zz_filesystem_generated.go go.mod schema/func_yaml-schema.json
TEMPLATES := $(shell find templates -name '*' -type f)

.PHONY: test docs

# Default Targets
all: build docs

# Help Text
# Headings: lines with `##$` comment prefix
# Targets:  printed if their line includes a `##` comment
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


###############
##@ Development
###############

build: $(BIN) ## (default) Build binary for current OS

$(BIN): $(CODE)
	env CGO_ENABLED=0 go build -ldflags $(LDFLAGS) ./cmd/$(BIN)

test: $(CODE) ## Run core unit tests
	go test -race -cover -coverprofile=coverage.txt ./...

check: bin/golangci-lint ## Check code quality (lint)
	./bin/golangci-lint run --timeout 300s
	cd test/_e2e && ../../bin/golangci-lint run --timeout 300s

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.49.0

.PHONY: clean_templates

clean_templates:
	# Removing temporary template files
	@rm -f templates/go/cloudevents/go.sum
	@rm -f templates/go/http/go.sum
	@rm -rf templates/node/cloudevents/node_modules
	@rm -rf templates/node/http/node_modules
	@rm -rf templates/python/cloudevents/__pycache__
	@rm -rf templates/python/http/__pycache__
	@rm -rf templates/typescript/cloudevents/node_modules
	@rm -rf templates/typescript/http/node_modules
	@rm -rf templates/rust/cloudevents/target
	@rm -rf templates/rust/http/target
	@rm -rf templates/quarkus/cloudevents/target
	@rm -rf templates/quarkus/http/target
	@rm -rf templates/springboot/cloudevents/target
	@rm -rf templates/springboot/http/target
	@rm -f templates/**/.DS_Store

.PHONY: zz_filesystem_generated.go

zz_filesystem_generated.go: clean_templates
	go generate filesystem.go

.PHONY: clean

clean: clean_templates ## Remove generated artifacts such as binaries and schemas
	rm -f $(BIN) $(BIN_WINDOWS) $(BIN_LINUX) $(BIN_DARWIN_AMD64) $(BIN_DARWIN_ARM64)
	rm -f schema/func_yaml-schema.json
	rm -f coverage.txt

docs:
	# Generating command reference doc
	go run docs/generator/main.go -ldflags $(LDFLAGS)

#############
##@ Templates
#############

test-templates: test-go test-node test-python test-quarkus test-springboot test-rust test-typescript ## Run all template tests

test-go: ## Test Go templates
	cd templates/go/cloudevents && go mod tidy && go test
	cd templates/go/http && go mod tidy && go test

test-node: ## Test Node templates
	cd templates/node/cloudevents && npm ci && npm test && rm -rf node_modules
	cd templates/node/http && npm ci && npm test && rm -rf node_modules

test-python: ## Test Python templates
	cd templates/python/cloudevents && pip3 install -r requirements.txt && python3 test_func.py && rm -rf __pycache__
	cd templates/python/http && python3 test_func.py && rm -rf __pycache__

test-quarkus: ## Test Quarkus templates
	cd templates/quarkus/cloudevents && mvn test && mvn clean
	cd templates/quarkus/http && mvn test && mvn clean

test-springboot: ## Test Spring Boot templates
	cd templates/springboot/cloudevents && mvn test && mvn clean
	cd templates/springboot/http && mvn test && mvn clean

test-rust: ## Test Rust templates
	cd templates/rust/cloudevents && cargo test && cargo clean
	cd templates/rust/http && cargo test && cargo clean

test-typescript: ## Test Typescript templates
	cd templates/typescript/cloudevents && npm ci && npm test && rm -rf node_modules build
	cd templates/typescript/http && npm ci && npm test && rm -rf node_modules build


###################
##@ Extended Testing (cluster required)
###################

test-integration: ## Run integration tests using an available cluster.
	go test -tags integration --coverprofile=coverage.txt ./... -v

test-e2e: ## Run end-to-end tests using an available cluster.
	./test/e2e_lifecycle_tests.sh node
	./test/e2e_extended_tests.sh

test-e2e-runtime: ## Run end-to-end lifecycle tests using an available cluster for a single runtime.
	./test/e2e_lifecycle_tests.sh $(runtime)

test-e2e-on-cluster: ## Run end-to-end on-cluster build tests using an available cluster.
	./test/e2e_oncluster_tests.sh

######################
##@ Release Artifacts
######################

cross-platform: darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 linux-ppc64le linux-s390x windows ## Build all distributable (cross-platform) binaries

darwin-arm64: $(BIN_DARWIN_ARM64) ## Build for mac M1

$(BIN_DARWIN_ARM64): zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BIN_DARWIN_ARM64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

darwin-amd64: $(BIN_DARWIN_AMD64) ## Build for Darwin (macOS)

$(BIN_DARWIN_AMD64): zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DARWIN_AMD64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

linux-amd64: $(BIN_LINUX_AMD64) ## Build for Linux amd64

$(BIN_LINUX_AMD64): zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_LINUX_AMD64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

linux-arm64: $(BIN_LINUX_ARM64) ## Build for Linux arm64

$(BIN_LINUX_ARM64): zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BIN_LINUX_ARM64) -ldflags $(LDFLAGS) ./cmd/$(BIN)

linux-ppc64le: $(BIN_LINUX_PPC64LE) ## Build for Linux ppc64le

$(BIN_LINUX_PPC64LE): zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -o $(BIN_LINUX_PPC64LE) -ldflags $(LDFLAGS) ./cmd/$(BIN)

linux-s390x: $(BIN_LINUX_S390X) ## Build for Linux s390x

$(BIN_LINUX_S390X): zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=s390x go build -o $(BIN_LINUX_S390X) -ldflags $(LDFLAGS) ./cmd/$(BIN)

windows: $(BIN_WINDOWS) ## Build for Windows

$(BIN_WINDOWS): zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_WINDOWS) -ldflags $(LDFLAGS) ./cmd/$(BIN)

######################
##@ Schemas
######################
schema-generate: schema/func_yaml-schema.json ## Generate func.yaml schema
schema/func_yaml-schema.json: function.go
	go run schema/generator/main.go

schema-check: ## Check that func.yaml schema is up-to-date
	mv schema/func_yaml-schema.json schema/func_yaml-schema-previous.json
	make schema-generate
	diff schema/func_yaml-schema.json schema/func_yaml-schema-previous.json ||\
	(echo "\n\nFunction config schema 'schema/func_yaml-schema.json' is obsolete, please run 'make schema-generate'.\n\n"; rm -rf schema/func_yaml-schema-previous.json; exit 1)
	rm -rf schema/func_yaml-schema-previous.json

