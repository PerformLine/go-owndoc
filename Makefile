.EXPORT_ALL_VARIABLES:

GO111MODULE ?= on
BIN         ?= bin/owndoc-$(shell go env GOOS)-$(shell go env GOARCH)

all: deps fmt test build

go.mod:
	go mod init github.com/ghetzel/go-owndoc

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
	cp $(@) ~/bin/owndoc

docs:
	$(BIN) generate

build: $(BIN)

.PHONY: deps docs all build fmt test $(BIN)