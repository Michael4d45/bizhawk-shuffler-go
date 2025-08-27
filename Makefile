BIN_DIR := bin
GO := go
GOOS := $(shell $(GO) env GOOS)
ifeq ($(GOOS),windows)
EXT := .exe
else
EXT :=
endif

.PHONY: all server client clean

all: server client

server:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/server/bizshuffle-server$(EXT) ./cmd/server
	@mkdir -p $(BIN_DIR)/server/web
	@cp -r web/* $(BIN_DIR)/server/web/ 2> /dev/null || true

client:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/client/bizshuffle-client$(EXT) ./cmd/client
	@cp server.lua $(BIN_DIR)/client/server.lua

clean:
	rm -rf $(BIN_DIR)
