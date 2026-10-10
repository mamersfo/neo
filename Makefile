.PHONY: build vet test fmt lint

VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o bin/neo .

vet:
	go vet ./...
	go vet -tags integration ./...

test:
	go test ./...

test-all:
	go test ./... && go test -tags integration ./internal/docker ./internal/migrate -run 'TestStartStopCycle|TestResetCycle|TestDiffRoundTrip'

fmt:
	gofmt -l .

lint: vet fmt
