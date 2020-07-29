REPO := quay.io/boson/faas
BIN  := faas

DARWIN=$(BIN)_darwin_amd64
LINUX=$(BIN)_linux_amd64
WINDOWS=$(BIN)_windows_amd64.exe

CODE := $(shell find . -name '*.go')
DATE := $(shell date -u +"%Y%m%dT%H%M%SZ")
HASH := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG := $(shell git tag --points-at HEAD)
VERS := $(shell [ -z $(VTAG) ] && echo 'tip' || echo $(VTAG) )

build: all
all: $(BIN)

cross-platform: $(DARWIN) $(LINUX) $(WINDOWS)

darwin: $(DARWIN) ## Build for Darwin (macOS)

linux: $(LINUX) ## Build for Linux

windows: $(WINDOWS) ## Build for Windows

$(BIN): $(CODE)  ## Build using environment defaults
	go build -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(DARWIN):
	env GOOS=darwin GOARCH=amd64 go build -v -o $(DARWIN) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(LINUX):
	env GOOS=linux GOARCH=amd64 go build -v -o $(LINUX) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(WINDOWS):
	env GOOS=windows GOARCH=amd64 go build -v -o $(WINDOWS) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

test:
	go test -cover -coverprofile=coverage.out ./...

image: Dockerfile
	docker build -t $(REPO):latest  \
	             -t $(REPO):$(VERS) \
	             -t $(REPO):$(HASH) \
	             -t $(REPO):$(DATE)-$(VERS)-$(HASH) .

release: build test
	go get -u github.com/git-chglog/git-chglog/cmd/git-chglog
	git-chglog --next-tag $(VTAG) -o CHANGELOG.md
	git commit -am "release: $(VTAG)"
	git tag $(VTAG)

push: image
	docker push $(REPO):$(VERS)
	docker push $(REPO):$(HASH)
	docker push $(REPO):$(DATE)-$(VERS)-$(HASH)

latest:
	# push the local 'latest' tag as the new public latest version
	# (run by CI only for releases)
	docker push $(REPO):latest

bin/golangci-lint:
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin v1.27.0

check: bin/golangci-lint
	./bin/golangci-lint run --enable=unconvert,prealloc,bodyclose

clean:
	rm -f $(WINDOWS) $(LINUX) $(DARWIN)
	-@rm -f coverage.out
