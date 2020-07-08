REPO := quay.io/boson/faas
BIN  := faas

CODE := $(shell find . -name '*.go')
DATE := $(shell date -u +"%Y%m%dT%H%M%SZ")
HASH := $(shell git rev-parse --short HEAD 2>/dev/null)
VTAG := $(shell git tag --points-at HEAD)
VERS := $(shell [ -z $(VTAG) ] && echo 'tip' || echo $(VTAG) )

all: $(BIN)
build: all

$(BIN): $(CODE)
	go build -ldflags "-X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

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
	-@rm -f $(BIN)
	-@rm -f coverage.out
