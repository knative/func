# ##
#
# Run 'make help' for a summary
#
# ##

# Binaries
BIN               := func
BIN_DARWIN_AMD64  ?= $(BIN)_darwin_amd64
BIN_DARWIN_ARM64  ?= $(BIN)_darwin_arm64
BIN_LINUX_AMD64   ?= $(BIN)_linux_amd64
BIN_LINUX_ARM64   ?= $(BIN)_linux_arm64
BIN_LINUX_PPC64LE ?= $(BIN)_linux_ppc64le
BIN_LINUX_S390X   ?= $(BIN)_linux_s390x
BIN_WINDOWS       ?= $(BIN)_windows_amd64.exe

# Utilities
BIN_GOLANGCI_LINT ?= "$(PWD)/bin/golangci-lint"

# Version
# A verbose version is built into the binary including a date stamp, git commit
# hash and the version tag of the current commit (semver) if it exists.
# If the current commit does not have a semver tag, 'tip' is used, unless there
# is a TAG environment variable. Precedence is git tag, environment variable, 'tip'
HASH         := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG         := $(shell git tag --points-at HEAD | head -1)
VTAG         := $(shell [ -z $(VTAG) ] && echo $(ETAG) || echo $(VTAG))
VERS         ?= $(shell git describe --tags --match 'v*')
KVER         ?= $(shell git describe --tags --match 'knative-*')

LDFLAGS      := -X knative.dev/func/pkg/app.vers=$(VERS) -X knative.dev/func/pkg/app.kver=$(KVER) -X knative.dev/func/pkg/app.hash=$(HASH)
ifneq ($(FUNC_REPO_REF),)
  LDFLAGS      += -X knative.dev/func/pkg/pipelines/tekton.FuncRepoRef=$(FUNC_REPO_REF)
endif
ifneq ($(FUNC_REPO_BRANCH_REF),)
  LDFLAGS      += -X knative.dev/func/pkg/pipelines/tekton.FuncRepoBranchRef=$(FUNC_REPO_BRANCH_REF)
endif

MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

.PHONY: test docs

# Default Targets
all: build docs
	@echo '🎉 Build process completed!'

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

.PHONY: $(BIN)
$(BIN): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" ./cmd/$(BIN)

.PHONY: test
test: generate/zz_filesystem_generated.go ## Run core unit tests
	go test -ldflags "$(LDFLAGS)" -race -cover -coverprofile=coverage.txt ./...

.PHONY: check
check: $(BIN_GOLANGCI_LINT) ## Check code quality (lint)
	$(BIN_GOLANGCI_LINT) run --timeout 300s
	cd test && $(BIN_GOLANGCI_LINT) run --timeout 300s

$(BIN_GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.55.2

.PHONY: generate/zz_filesystem_generated.go
generate/zz_filesystem_generated.go: clean_templates templates/certs/ca-certificates.crt
	go generate pkg/functions/templates_embedded.go

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
	@rm -rf templates/typescript/cloudevents/build
	@rm -rf templates/typescript/http/build
	@rm -rf templates/rust/cloudevents/target
	@rm -rf templates/rust/http/target
	@rm -rf templates/quarkus/cloudevents/target
	@rm -rf templates/quarkus/http/target
	@rm -rf templates/springboot/cloudevents/target
	@rm -rf templates/springboot/http/target
	@rm -f templates/**/.DS_Store

.PHONY: clean
clean: clean_templates ## Remove generated artifacts such as binaries and schemas
	rm -f $(BIN) $(BIN_WINDOWS) $(BIN_LINUX) $(BIN_DARWIN_AMD64) $(BIN_DARWIN_ARM64)
	rm -f $(BIN_GOLANGCI_LINT)
	rm -f schema/func_yaml-schema.json
	rm -f coverage.txt

.PHONY: docs
docs:
	# Generating command reference doc
	go run docs/generator/main.go

#############
##@ Prow Integration
#############

presubmit-unit-tests: ## Run prow presubmit unit tests locally
	docker run --platform linux/amd64 -it --rm -v$(MAKEFILE_DIR):/src/ us-docker.pkg.dev/knative-tests/images/prow-tests:v20230616-086ddd644 sh -c 'cd /src && runner.sh ./test/presubmit-tests.sh --unit-tests'


#############
##@ Templates
#############

# TODO: add linters for other templates
.PHONY: check-templates
check-templates: check-go check-rust ## Run template source code checks

.PHONY: check-go
check-go: ## Check Go templates' source
	cd templates/go/scaffolding/instanced-http && go vet ./... &&  $(BIN_GOLANGCI_LINT) run
	cd templates/go/scaffolding/instanced-cloudevents && go vet && $(BIN_GOLANGCI_LINT) run
	cd templates/go/scaffolding/static-http && go vet ./... && $(BIN_GOLANGCI_LINT) run
	cd templates/go/scaffolding/static-cloudevents && go vet ./... && $(BIN_GOLANGCI_LINT) run

.PHONY: check-rust
check-rust: ## Check Rust templates' source
	cd templates/rust/cloudevents && cargo clippy && cargo clean
	cd templates/rust/http && cargo clippy && cargo clean

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
	cd templates/quarkus/cloudevents && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar
	cd templates/quarkus/http && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar

test-springboot: ## Test Spring Boot templates
	cd templates/springboot/cloudevents && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar
	cd templates/springboot/http && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar

test-rust: ## Test Rust templates
	cd templates/rust/cloudevents && cargo -q test && cargo clean
	cd templates/rust/http && cargo -q test && cargo clean

test-typescript: ## Test Typescript templates
	cd templates/typescript/cloudevents && npm ci && npm test && rm -rf node_modules build
	cd templates/typescript/http && npm ci && npm test && rm -rf node_modules build

###############
##@ Scaffolding
###############

# Pulls runtimes then rebuilds the embedded filesystem
update-runtimes:  pull-runtimes generate/zz_filesystem_generated.go ## Update Scaffolding Runtimes

pull-runtimes:
	cd templates/go/scaffolding/instanced-http && go get -u knative.dev/func-go/http
	cd templates/go/scaffolding/static-http && go get -u knative.dev/func-go/http
	cd templates/go/scaffolding/instanced-cloudevents && go get -u knative.dev/func-go/cloudevents
	cd templates/go/scaffolding/static-cloudevents && go get -u knative.dev/func-go/cloudevents

.PHONY: cert
certs: templates/certs/ca-certificates.crt ## Update root certificates

.PHONY: templates/certs/ca-certificates.crt
templates/certs/ca-certificates.crt:
	# Updating root certificates
	curl --output templates/certs/ca-certificates.crt https://curl.se/ca/cacert.pem

###################
##@ Extended Testing (cluster required)
###################

test-integration: ## Run integration tests using an available cluster.
	go test -ldflags "$(LDFLAGS)" -tags integration -timeout 30m --coverprofile=coverage.txt ./... -v

.PHONY: func-instrumented

func-instrumented: ## Func binary that is instrumented for e2e tests
	env CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -cover -o func ./cmd/$(BIN)

test-e2e: func-instrumented ## Run end-to-end tests using an available cluster.
	./test/e2e_extended_tests.sh

test-e2e-runtime: func-instrumented ## Run end-to-end lifecycle tests using an available cluster for a single runtime.
	./test/e2e_lifecycle_tests.sh $(runtime)

test-e2e-on-cluster: func-instrumented ## Run end-to-end on-cluster build tests using an available cluster.
	./test/e2e_oncluster_tests.sh

######################
##@ Release Artifacts
######################

cross-platform: darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 linux-ppc64le linux-s390x windows ## Build all distributable (cross-platform) binaries

darwin-arm64: $(BIN_DARWIN_ARM64) ## Build for mac M1

$(BIN_DARWIN_ARM64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BIN_DARWIN_ARM64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

darwin-amd64: $(BIN_DARWIN_AMD64) ## Build for Darwin (macOS)

$(BIN_DARWIN_AMD64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DARWIN_AMD64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

linux-amd64: $(BIN_LINUX_AMD64) ## Build for Linux amd64

$(BIN_LINUX_AMD64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_LINUX_AMD64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

linux-arm64: $(BIN_LINUX_ARM64) ## Build for Linux arm64

$(BIN_LINUX_ARM64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BIN_LINUX_ARM64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

linux-ppc64le: $(BIN_LINUX_PPC64LE) ## Build for Linux ppc64le

$(BIN_LINUX_PPC64LE): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -o $(BIN_LINUX_PPC64LE) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

linux-s390x: $(BIN_LINUX_S390X) ## Build for Linux s390x

$(BIN_LINUX_S390X): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=s390x go build -o $(BIN_LINUX_S390X) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

windows: $(BIN_WINDOWS) ## Build for Windows

$(BIN_WINDOWS): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_WINDOWS) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

######################
##@ Schemas
######################

schema-generate: schema/func_yaml-schema.json ## Generate func.yaml schema
schema/func_yaml-schema.json: pkg/functions/function.go pkg/functions/function_*.go
	go run schema/generator/main.go

schema-check: ## Check that func.yaml schema is up-to-date
	mv schema/func_yaml-schema.json schema/func_yaml-schema-previous.json
	make schema-generate
	diff schema/func_yaml-schema.json schema/func_yaml-schema-previous.json ||\
	(echo "\n\nFunction config schema 'schema/func_yaml-schema.json' is obsolete, please run 'make schema-generate'.\n\n"; rm -rf schema/func_yaml-schema-previous.json; exit 1)
	rm -rf schema/func_yaml-schema-previous.json

