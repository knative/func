REPO := quay.io/boson/func

BIN     := func
DARWIN  :=$(BIN)_darwin_amd64
LINUX   :=$(BIN)_linux_amd64
WINDOWS :=$(BIN)_windows_amd64.exe

CODE := $(shell find . -name '*.go')
DATE := $(shell date -u +"%Y%m%dT%H%M%SZ")
HASH := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG := $(shell git tag --points-at HEAD)
# a VERS environment variable takes precedence over git tags
# and is necessary with release-please-action which tags asynchronously
# unless explicitly, synchronously tagging as is done in ci.yaml
VERS ?= $(shell [ -z $(VTAG) ] && echo 'tip' || echo $(VTAG) )

LDFLAGS := -X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)

build: all
all: $(BIN)

templates.tgz:
	# ensure no cached dependencies are added to the binary
	rm -rf templates/node/events/node_modules
	rm -rf templates/node/http/node_modules
	rm -rf templates/python/events/__pycache__
	rm -rf templates/python/http/__pycache__
	# see generate.go for details
	go generate

cross-platform: $(DARWIN) $(LINUX) $(WINDOWS)

darwin: $(DARWIN) ## Build for Darwin (macOS)

linux: $(LINUX) ## Build for Linux

windows: $(WINDOWS) ## Build for Windows

$(BIN): templates.tgz $(CODE)  ## Build using environment defaults
	env CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" ./cmd/$(BIN)

$(DARWIN): templates.tgz
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(DARWIN) -ldflags "$(LDFLAGS)" ./cmd/$(BIN)

$(LINUX): templates.tgz
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(LINUX) -ldflags "$(LDFLAGS)" ./cmd/$(BIN)

$(WINDOWS): templates.tgz
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(WINDOWS) -ldflags "$(LDFLAGS)" ./cmd/$(BIN)

test: test-binary test-node test-python test-quarkus test-go

test-binary: templates.tgz
	go test -race -cover -coverprofile=coverage.out ./...

test-node:
	cd templates/node/events && npm ci && npm test && rm -rf node_modules
	cd templates/node/http && npm ci && npm test && rm -rf node_modules

test-python:
	cd templates/python/events && pip3 install -r requirements.txt && python3 test_func.py
	cd templates/python/http && python3 test_func.py

test-quarkus:
	cd templates/quarkus/events && mvn test && mvn clean
	cd templates/quarkus/http && mvn test && mvn clean

test-go:
	cd templates/go/events && go test
	cd templates/go/http && go test

test-integration: templates.tgz
	go test -tags integration ./...

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.28.0

check: bin/golangci-lint
	./bin/golangci-lint run --timeout 300s

release: build test
	go get -u github.com/git-chglog/git-chglog/cmd/git-chglog
	git-chglog --next-tag $(VTAG) -o CHANGELOG.md
	git commit -am "release: $(VTAG)"
	git tag $(VTAG)

cluster: ## Set up a local cluster for integraiton tests.
	# Creating KinD cluster `kind`.
	# Delete with ./hack/delete.sh
	./hack/allocate.sh && ./hack/configure.sh

clean:
	rm -f $(BIN) $(WINDOWS) $(LINUX) $(DARWIN)
	rm -f templates.tgz
	-rm -f coverage.out
