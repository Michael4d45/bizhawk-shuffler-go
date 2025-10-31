BIN_DIR := dist
GO := go
GOOS := $(shell $(GO) env GOOS)
ifeq ($(GOOS),windows)
EXT := .exe
else
EXT :=
endif

.PHONY: all server client installer clean

all: server client installer

server:
	@mkdir -p $(BIN_DIR)/server
	$(GO) build -o $(BIN_DIR)/server/bizshuffle-server$(EXT) ./cmd/server
	@mkdir -p $(BIN_DIR)/server/web
	@cp -r web/* $(BIN_DIR)/server/web/ 2> /dev/null || true
	@mkdir -p $(BIN_DIR)/server/plugins
	@cp -r plugins/* $(BIN_DIR)/server/plugins/ 2> /dev/null || true

client:
	@mkdir -p $(BIN_DIR)/client
	# generate .syso for Windows only
	$(GO) build -o $(BIN_DIR)/client/bizshuffle-client$(EXT) ./cmd/client
	@cp server.lua $(BIN_DIR)/client/server.lua 2> /dev/null || true
	@cp bizshuffle-client.ico $(BIN_DIR)/client/bizshuffle-client.ico 2> /dev/null || true

installer:
	@mkdir -p $(BIN_DIR)/installer
	$(GO) build -o $(BIN_DIR)/installer/bizshuffle-installer$(EXT) ./cmd/installer

clean:
	rm -rf $(BIN_DIR)