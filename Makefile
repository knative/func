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
all: $(LINUX)
cross-platform: $(DARWIN) $(LINUX) $(WINDOWS)

darwin: $(DARWIN) ## Build for Darwin (macOS)

linux: $(LINUX) ## Build for Linux

windows: $(WINDOWS) ## Build for Windows

$(DARWIN):
	env GOOS=darwin GOARCH=amd64 go build -v -o $(DARWIN) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(LINUX):
	env GOOS=linux GOARCH=amd64 go build -v -o $(LINUX) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

$(WINDOWS):
	env GOOS=windows GOARCH=amd64 go build -v -o $(WINDOWS) -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

# $(BIN): $(CODE)
# 	go build -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

test:
	go test -cover -coverprofile=coverage.out ./...

image: Dockerfile
	docker build -t $(REPO):$(VERS) \
	             -t $(REPO):$(HASH) \
	             -t $(REPO):$(DATE)-$(VERS)-$(HASH) .

push: image
	docker push $(REPO):$(VERS)
	docker push $(REPO):$(HASH)
	docker push $(REPO):$(DATE)-$(VERS)-$(HASH)

clean:
	rm -f $(WINDOWS) $(LINUX) $(DARWIN)
	-@rm -f coverage.out
