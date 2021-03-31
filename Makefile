REPO := quay.io/boson/func
BIN  := func

PKGER?=pkger

DARWIN=$(BIN)_darwin_amd64
LINUX=$(BIN)_linux_amd64
WINDOWS=$(BIN)_windows_amd64.exe

CODE := $(shell find . -name '*.go')
DATE := $(shell date -u +"%Y%m%dT%H%M%SZ")
HASH := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG := $(shell git tag --points-at HEAD)
# a VERS environment variable takes precedence over git tags
# due to issues with release-please-action tagging not working
# as expected in CI
VERS ?= $(shell [ -z $(VTAG) ] && echo 'tip' || echo $(VTAG) )

TEMPLATE_DIRS=$(shell find templates -type d)
TEMPLATE_FILES=$(shell find templates -type f -name '*')
TEMPLATE_PACKAGE=pkged.go

build: all
all: $(TEMPLATE_PACKAGE) $(BIN)

$(TEMPLATE_PACKAGE): templates $(TEMPLATE_DIRS) $(TEMPLATE_FILES)
  # ensure no cached dependencies are added to the binary
	rm -rf templates/node/events/node_modules
	rm -rf templates/node/http/node_modules
	rm -rf templates/python/events/__pycache__
	rm -rf templates/python/http/__pycache__
	# to install pkger:  go get github.com/markbates/pkger/cmd/pkger
	$(PKGER)

cross-platform: $(TEMPLATE_PACKAGE) $(DARWIN) $(LINUX) $(WINDOWS)

darwin: $(DARWIN) ## Build for Darwin (macOS)

linux: $(LINUX) ## Build for Linux

windows: $(WINDOWS) ## Build for Windows

$(BIN): $(CODE)  ## Build using environment defaults
	env CGO_ENABLED=0 go build -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(DARWIN):
	env CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o $(DARWIN) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(LINUX):
	env CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(LINUX) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(WINDOWS):
	env CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o $(WINDOWS) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

test: test-binary test-node test-python test-quarkus test-go

test-binary:
	go test -race -cover -coverprofile=coverage.out ./...

test-node:
	cd templates/node/events && npm ci && npm test && rm -rf node_modules
	cd templates/node/http && npm ci && npm test && rm -rf node_modules

test-python:
	cd templates/python/events && pip3 install -r requirements.txt && python3 test_func.py
	cd templates/python/http && python3 test_func.py

test-quarkus:
	cd templates/quarkus/events && mvn test
	cd templates/quarkus/http && mvn test

test-go:
	cd templates/go/events && go test
	cd templates/go/http && go test

test-integration:
	go test -tags integration ./...

image: Dockerfile
	docker build -t $(REPO):latest  \
	             -t $(REPO):$(VERS) \
	             -t $(REPO):$(HASH) \
	             -t $(REPO):$(DATE)-$(VERS)-$(HASH) .

push: image
	docker push $(REPO):$(VERS)
	docker push $(REPO):$(HASH)
	docker push $(REPO):$(DATE)-$(VERS)-$(HASH)

latest:
	# push the local 'latest' tag as the new public latest version
	# (run by CI only for releases)
	docker push $(REPO):latest

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
	-rm -f coverage.out
