BINARY ?= shh
CMD ?= ./cmd/shh
BIN_DIR ?= bin

.PHONY: build run test fmt vet tidy clean

build:
	@mkdir -p $(BIN_DIR)
	GO111MODULE=on go build -o $(BIN_DIR)/$(BINARY) $(CMD)

run:
	GO111MODULE=on go run $(CMD)

test:
	GO111MODULE=on go test ./...

fmt:
	GO111MODULE=on go fmt ./...

vet:
	GO111MODULE=on go vet ./...

tidy:
	GO111MODULE=on go mod tidy

clean:
	@rm -rf $(BIN_DIR)
