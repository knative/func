REPO := quay.io/boson/faas
BIN  := faas

CODE := $(shell find . -name '*.go')
DATE := $(shell date -u +"%Y%m%dT%H%M%SZ")
HASH := $(shell git rev-parse --short HEAD 2>/dev/null)
BRCH := $(shell git symbolic-ref --short -q HEAD | sed 's/\//-/g')
VTAG := $(shell git tag --points-at HEAD)
VERS := $(shell [ -z $(VTAG) ] && echo 'tip' || echo $(VTAG) )

all: $(BIN)

$(BIN): $(CODE)
	go build -ldflags "-X main.brch=$(BRCH) -X main.date=$(DATE) -X main.vers=$(VERS) -X main.hash=$(HASH)" ./cmd/$(BIN)

test:
	go test -cover -coverprofile=coverage.out ./...

image: Dockerfile
	docker build -t $(REPO):$(BRCH) \
	             -t $(REPO):$(VERS) \
	             -t $(REPO):$(HASH) \
	             -t $(REPO):$(BRCH)-$(DATE)-$(VERS)-$(HASH) .

push: image
	docker push $(REPO):$(BRCH)
	docker push $(REPO):$(VERS)
	docker push $(REPO):$(HASH)
	docker push $(REPO):$(BRCH)-$(DATE)-$(VERS)-$(HASH)

clean:
	-@rm -f $(BIN)
	-@rm -f coverage.out
