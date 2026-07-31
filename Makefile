VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
LDFLAGS := -s -w \
	-X github.com/SpecFlowdev/AmneziaX/internal/version.Version=$(VERSION) \
	-X github.com/SpecFlowdev/AmneziaX/internal/version.Commit=$(COMMIT)

.PHONY: all ui panel node build test lint proto clean docker

all: build

## ui: compile the SPA straight into the package the panel embeds
ui:
	cd frontend && npm ci --no-audit --no-fund && npm run build

## panel: build the panel binary (run `make ui` first for a bundled UI)
panel:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/amneziax-panel ./cmd/panel

## node: build the node agent binary
node:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/amneziax-node ./cmd/node

build: panel node

test:
	go test ./...

lint:
	gofmt -l . | tee /dev/stderr | (! read)
	go vet ./...

## proto: regenerate the panel <-> node gRPC contract
proto:
	protoc --proto_path=proto \
		--go_out=gen/go --go_opt=paths=source_relative \
		--go-grpc_out=gen/go --go-grpc_opt=paths=source_relative \
		proto/node/v1/node.proto

docker:
	docker build -f deploy/Dockerfile.panel -t amneziax-panel:$(VERSION) --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) .
	docker build -f deploy/Dockerfile.node  -t amneziax-node:$(VERSION)  --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) .

clean:
	rm -rf bin internal/webui/dist/assets
