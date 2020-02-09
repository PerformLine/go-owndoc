.EXPORT_ALL_VARIABLES:

GO111MODULE ?= on
BIN         ?= bin/godocfriend-$(shell go env GOOS)-$(shell go env GOARCH)

all: deps fmt test build

go.mod:
	go mod init github.com/ghetzel/godocfriend

deps: go.mod
	go get ./...

$(LOCALS):
	gofmt -w $(@)

fmt: $(LOCALS)
	go generate -x ./...
	go vet ./...

test:
	go test ./...

$(BIN):
	go build -o $(@) .
	cp $(@) ~/bin/godocfriend

build: $(BIN)

.PHONY: deps all build fmt test $(BIN)