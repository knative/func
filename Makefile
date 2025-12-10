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
BIN_MISSPELL      ?= "$(PWD)/bin/misspell"
BIN_GOIMPORTS     ?= "$(PWD)/bin/goimports"

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

LDFLAGS      := -X knative.dev/func/pkg/version.Vers=$(VERS) -X knative.dev/func/pkg/version.Kver=$(KVER) -X knative.dev/func/pkg/version.Hash=$(HASH)

FUNC_UTILS_IMG ?= ghcr.io/knative/func-utils:v2
LDFLAGS += -X knative.dev/func/pkg/k8s.SocatImage=$(FUNC_UTILS_IMG)
LDFLAGS += -X knative.dev/func/pkg/k8s.TarImage=$(FUNC_UTILS_IMG)
LDFLAGS += -X knative.dev/func/pkg/pipelines/tekton.FuncUtilImage=$(FUNC_UTILS_IMG)

GOFLAGS      := "-ldflags=$(LDFLAGS)"
export GOFLAGS

MAKEFILE_DIR := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))

# Disable output buffering (stream)
MAKEFLAGS += --output-sync=none

# Default Targets
.PHONY: all
all: build docs
	@echo '🎉 Build process completed!'

# Help Text
# Headings: lines with `##$` comment prefix
# Targets:  printed if their line includes a `##` comment
.PHONY: help
help:
	@echo 'Usage: make <OPTIONS> ... <TARGETS>'
	@echo ''
	@echo 'Available targets are:'
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z0-9_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)


###############
##@ Development
###############

.PHONY: build
build: $(BIN) ## (default) Build binary for current OS

.PHONY: $(BIN)
$(BIN): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 go build ./cmd/$(BIN)

.PHONY: test
test: generate/zz_filesystem_generated.go ## Run core unit tests
	go test -race -cover -coverprofile=coverage.txt ./...

.PHONY: check
check: check-lint check-goimports check-misspell check-whitespace check-eof ## Check code quality (comprehensive)

.PHONY: check-lint
check-lint: $(BIN_GOLANGCI_LINT) ## Run golangci-lint
	$(BIN_GOLANGCI_LINT) run --timeout 300s

.PHONY: check-goimports
check-goimports: $(BIN_GOIMPORTS) ## Check Go import formatting
	@echo "Checking Go import formatting..."
	@git ls-files | \
		git check-attr --stdin linguist-generated | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		git check-attr --stdin linguist-vendored | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		grep -Ev '(vendor/|third_party/|\.git)' | \
		grep '\.go$$' | \
		while IFS= read -r file; do [ -f "$$file" ] && echo "$$file"; done | \
		xargs $(BIN_GOIMPORTS) -l | grep . && \
		(echo "Error: Files with incorrect import formatting found. Run 'goimports -w <file>' to fix."; exit 1) || true

.PHONY: check-misspell
check-misspell: $(BIN_MISSPELL) ## Check for common misspellings
	@echo "Checking for misspellings..."
	@git ls-files | \
		git check-attr --stdin linguist-generated | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		git check-attr --stdin linguist-vendored | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		grep -Ev '(vendor/|third_party/|\.git)' | \
		grep -v '\.svg$$' | \
		while IFS= read -r file; do [ -f "$$file" ] && echo "$$file"; done | \
		xargs $(BIN_MISSPELL) -i importas -error

.PHONY: check-whitespace
check-whitespace: ## Check for trailing whitespace
	@echo "Checking for trailing whitespace..."
	@git ls-files | \
		git check-attr --stdin linguist-generated | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		git check-attr --stdin linguist-vendored | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		grep -Ev '(vendor/|third_party/|\.git)' | \
		grep -v '\.svg$$' | \
		while IFS= read -r file; do [ -f "$$file" ] && echo "$$file"; done | \
		xargs grep -nE " +$$" 2>&1 | grep -v "Binary file" && \
		(echo "Error: Trailing whitespace found"; exit 1) || true

.PHONY: check-eof
check-eof: ## Check files end with newlines
	@echo "Checking for missing EOF newlines..."
	@git ls-files | \
		git check-attr --stdin linguist-generated | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		git check-attr --stdin linguist-vendored | grep -Ev ': (set|true)$$' | cut -d: -f1 | \
		grep -Ev '(vendor/|third_party/|\.git)' | \
		grep -Ev '\.(ai|svg)$$' | \
		while IFS= read -r file; do \
			if [ -f "$$file" ] && [ -n "$$(tail -c 1 "$$file" 2>/dev/null)" ]; then \
				echo "$$file: missing newline at EOF"; \
			fi; \
		done | grep . && exit 1 || true

$(BIN_GOLANGCI_LINT):
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v2.5.0

$(BIN_MISSPELL):
	@echo "Installing misspell..."
	@GOBIN=$(PWD)/bin go install github.com/client9/misspell/cmd/misspell@latest

$(BIN_GOIMPORTS):
	@echo "Installing goimports..."
	@GOBIN=$(PWD)/bin go install golang.org/x/tools/cmd/goimports@latest

.PHONY: generate/zz_filesystem_generated.go
generate/zz_filesystem_generated.go: clean_templates
	go generate pkg/functions/templates_embedded.go

.PHONY: clean_templates
clean_templates:
	# Removing temporary template files
	@rm -rf templates/**/.DS_Store
	@rm -rf templates/node/cloudevents/node_modules
	@rm -rf templates/node/http/node_modules
	@rm -rf templates/python/cloudevents/.venv
	@rm -rf templates/python/cloudevents/.pytest_cache
	@rm -rf templates/python/cloudevents/function/__pycache__
	@rm -rf templates/python/cloudevents/tests/__pycache__
	@rm -rf templates/python/http/.venv
	@rm -rf templates/python/http/.pytest_cache
	@rm -rf templates/python/http/function/__pycache__
	@rm -rf templates/python/http/tests/__pycache__
	@rm -rf templates/quarkus/cloudevents/target
	@rm -rf templates/quarkus/http/target
	@rm -rf templates/rust/cloudevents/target
	@rm -rf templates/rust/http/target
	@rm -rf templates/springboot/cloudevents/target
	@rm -rf templates/springboot/http/target
	@rm -rf templates/typescript/cloudevents/build
	@rm -rf templates/typescript/cloudevents/node_modules
	@rm -rf templates/typescript/http/build
	@rm -rf templates/typescript/http/node_modules

.PHONY: clean
clean: clean_templates ## Remove generated artifacts such as binaries and schemas
	rm -f $(BIN) $(BIN_WINDOWS) $(BIN_LINUX) $(BIN_DARWIN_AMD64) $(BIN_DARWIN_ARM64)
	rm -f $(BIN_GOLANGCI_LINT)
	rm -f schema/func_yaml-schema.json
	rm -f coverage.txt

.PHONY: docs
docs:
	# Generating command reference doc
	KUBECONFIG="$(shell mktemp)" go run docs/generator/main.go

#############
##@ Prow Integration
#############

.PHONY: presubmit-unit-tests
presubmit-unit-tests: ## Run prow presubmit unit tests locally
	docker run --platform linux/amd64 -it --rm -v$(MAKEFILE_DIR):/src/ us-docker.pkg.dev/knative-tests/images/prow-tests:v20230616-086ddd644 sh -c 'cd /src && runner.sh ./test/presubmit-tests.sh --unit-tests'


#############
##@ Templates
#############

.PHONY: check-embedded-fs
check-embedded-fs: ## Check the embedded templates FS
	go test -run "^\QTestFileSystems\E$$/^\Qembedded\E$$" ./pkg/filesystem

# TODO: add linters for other templates
.PHONY: check-templates
check-templates: check-go check-rust check-typescript ## Run template source code checks

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

.PHONY: check-typescript
check-typescript: ## Check TypeScript templates' source
	cd templates/typescript/cloudevents && npm ci && npx eslint --ext .ts . && rm -rf node_modules build
	cd templates/typescript/http && npm ci && npx eslint --ext .ts . && rm -rf node_modules build

.PHONY: test-templates
test-templates: test-go test-node test-python test-quarkus test-springboot test-rust test-typescript ## Run all template tests

.PHONY: test-go
test-go: ## Test Go templates
	cd templates/go/cloudevents && go mod tidy && go test
	cd templates/go/http && go mod tidy && go test

.PHONY: test-node
test-node: ## Test Node templates
	cd templates/node/cloudevents && npm ci && npm test && rm -rf node_modules
	cd templates/node/http && npm ci && npm test && rm -rf node_modules

.PHONY: test-python
test-python: ## Test Python templates
	./hack/test-python.sh

.PHONY: test-quarkus
test-quarkus: ## Test Quarkus templates
	cd templates/quarkus/cloudevents && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar
	cd templates/quarkus/http && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar

.PHONY: test-springboot
test-springboot: ## Test Spring Boot templates
	cd templates/springboot/cloudevents && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar
	cd templates/springboot/http && ./mvnw -q test && ./mvnw clean && rm .mvn/wrapper/maven-wrapper.jar

.PHONY: test-rust
test-rust: ## Test Rust templates
	cd templates/rust/cloudevents && cargo -q test && cargo clean
	cd templates/rust/http && cargo -q test && cargo clean

.PHONY: test-typescript
test-typescript: ## Test Typescript templates
	cd templates/typescript/cloudevents && npm ci && npm test && rm -rf node_modules build
	cd templates/typescript/http && npm ci && npm test && rm -rf node_modules build

###############
##@ Scaffolding
###############

# Pulls runtimes then rebuilds the embedded filesystem
.PHONY: update-runtimes
update-runtimes:  update-runtime-go generate/zz_filesystem_generated.go ## Update Scaffolding Runtimes

.PHONY: update-runtime-go
update-runtime-go:
	@echo "Updating Go runtime..."
	cd templates/go/scaffolding/instanced-http && go get -u knative.dev/func-go/http
	cd templates/go/scaffolding/static-http && go get -u knative.dev/func-go/http
	cd templates/go/scaffolding/instanced-cloudevents && go get -u knative.dev/func-go/cloudevents
	cd templates/go/scaffolding/static-cloudevents && go get -u knative.dev/func-go/cloudevents


.PHONY: certs
certs: templates/certs/ca-certificates.crt ## Update root certificates

.PHONY: templates/certs/ca-certificates.crt
templates/certs/ca-certificates.crt:
	# Updating root certificates
	curl --output templates/certs/ca-certificates.crt https://curl.se/ca/cacert.pem

###################
##@ Extended Testing (cluster required)
###################

# See target "test" for unit tests only

.PHONY: test-integration
test-integration: ## Run integration tests using an available cluster.
	go test -cover -coverprofile=coverage.txt -tags integration -timeout 60m ./... -v -run TestInt_

.PHONY: test-e2e
test-e2e: func-instrumented-bin ## Basic E2E tests (includes core, metadata and remote tests)
	# Runtime and other options can be configured using the FUNC_E2E_* environment variables. see e2e_test.go
	go test -tags e2e -timeout 60m ./e2e -v -run "TestCore_|TestMetadata_|TestRemote_"
	go tool covdata textfmt -i=$${FUNC_E2E_GOCOVERDIR:-.coverage} -o coverage.txt


.PHONY: test-e2e-podman
test-e2e-podman: func-instrumented-bin ## Run E2E Podman-specific tests
	# see e2e_test.go for available options
	FUNC_E2E_PODMAN=true go test -tags e2e -timeout 60m ./e2e -v -run TestPodman_
	go tool covdata textfmt -i=$${FUNC_E2E_GOCOVERDIR:-.coverage} -o coverage.txt

test-e2e-matrix: func-instrumented-bin ## Basic E2E tests (includes core, metadata and remote tests)
	# Runtime and other options can be configured using the FUNC_E2E_* environment variables. see e2e_test.go
	FUNC_E2E_MATRIX=true go test -tags e2e -timeout 120m ./e2e -v -run TestMatrix_
	go tool covdata textfmt -i=$${FUNC_E2E_GOCOVERDIR:-.coverage} -o coverage.txt


.PHONY: test-full
test-full: func-instrumented-bin ## Run full test suite with all checks enabled
	./hack/test-full.sh

test-full-logged: func-instrumented-bin ## Run full test and log with timestamps (requires python)
	./hack/test-full.sh 2>&1 | python -u -c "import sys; from datetime import datetime; [print(f'[{datetime.now().strftime(\"%H:%M:%S\")}] {line}', end='', flush=True) for line in sys.stdin]" | tee ./test-full.log
	@echo '🎉 Full Test Complete.  Log stored in test-full.log'

.PHONY: func-instrumented-bin
func-instrumented-bin: # func binary instrumented with coverage reporting
	env CGO_ENABLED=1 go build -cover -o func ./cmd/$(BIN)


######################
##@ Release Artifacts
######################

.PHONY: cross-platform
cross-platform: darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 linux-ppc64le linux-s390x windows ## Build all distributable (cross-platform) binaries

.PHONY: darwin-arm64
darwin-arm64: $(BIN_DARWIN_ARM64) ## Build for mac M1

$(BIN_DARWIN_ARM64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -o $(BIN_DARWIN_ARM64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

.PHONY: darwn-amd64
darwin-amd64: $(BIN_DARWIN_AMD64) ## Build for Darwin (macOS)

$(BIN_DARWIN_AMD64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(BIN_DARWIN_AMD64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

.PHONY: linux-amd64
linux-amd64: $(BIN_LINUX_AMD64) ## Build for Linux amd64

$(BIN_LINUX_AMD64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BIN_LINUX_AMD64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

.PHONY: linux-arm64
linux-arm64: $(BIN_LINUX_ARM64) ## Build for Linux arm64

$(BIN_LINUX_ARM64): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o $(BIN_LINUX_ARM64) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

.PHONY: linux-ppc64le
linux-ppc64le: $(BIN_LINUX_PPC64LE) ## Build for Linux ppc64le

$(BIN_LINUX_PPC64LE): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=ppc64le go build -o $(BIN_LINUX_PPC64LE) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

.PHONY: linux-s390x
linux-s390x: $(BIN_LINUX_S390X) ## Build for Linux s390x

$(BIN_LINUX_S390X): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=linux GOARCH=s390x go build -o $(BIN_LINUX_S390X) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

.PHONY: windows
windows: $(BIN_WINDOWS) ## Build for Windows

$(BIN_WINDOWS): generate/zz_filesystem_generated.go
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(BIN_WINDOWS) -trimpath -ldflags "$(LDFLAGS) -w -s" ./cmd/$(BIN)

######################
##@ Schemas
######################

.PHONY: schema-generate
schema-generate: schema/func_yaml-schema.json ## Generate func.yaml schema
schema/func_yaml-schema.json: pkg/functions/function.go pkg/functions/function_*.go
	go run schema/generator/main.go

.PHONY: check-schema
check-schema: ## Check that func.yaml schema is up-to-date
	mv schema/func_yaml-schema.json schema/func_yaml-schema-previous.json
	make schema-generate
	diff schema/func_yaml-schema.json schema/func_yaml-schema-previous.json ||\
	(echo "\n\nFunction config schema 'schema/func_yaml-schema.json' is obsolete, please run 'make schema-generate'.\n\n"; rm -rf schema/func_yaml-schema-previous.json; exit 1)
	rm -rf schema/func_yaml-schema-previous.json

######################
##@ Hack scripting
######################

.PHONY: hack-generate-components
hack-generate-components: ## Regenerate components in hack/ dir
	cd hack && go run ./cmd/components

.PHONY: test-hack
test-hack:
	cd hack && go test ./... -v

## This is used by workflows

.PHONY: update-builder
__update-builder: # Used in automation
	cd hack && go run ./cmd/update-builder

.PHONY: setup-githooks
setup-githooks:
	git config --local core.hooksPath .githooks/
