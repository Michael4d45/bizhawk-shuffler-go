BIN_DIR := dist
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
	@mkdir -p $(BIN_DIR)/server
	$(GO) build -o $(BIN_DIR)/server/bizshuffle-server$(EXT) ./cmd/server
	@mkdir -p $(BIN_DIR)/server/web
	@cp -r web/* $(BIN_DIR)/server/web/ 2> /dev/null || true
	@mkdir -p $(BIN_DIR)/server/plugins
	@cp -r plugins/* $(BIN_DIR)/server/plugins/ 2> /dev/null || true

client:
	@mkdir -p $(BIN_DIR)/client
	# generate .syso for Windows only
ifeq ($(GOOS),windows)
	@rsrc -ico bizshuffle-client.ico -o cmd/client/bizshuffle-client.syso
endif
	$(GO) build -o $(BIN_DIR)/client/bizshuffle-client$(EXT) ./cmd/client
	@cp server.lua $(BIN_DIR)/client/server.lua 2> /dev/null || true
	@cp bizshuffle-client.ico $(BIN_DIR)/client/bizshuffle-client.ico 2> /dev/null || true
ifeq ($(GOOS),windows)
	# clean up .syso
	@rm -f cmd/client/bizshuffle-client.syso
endif

msi: client
	@mkdir -p $(BIN_DIR)/client
	candle "-dVersion=1.0.0" -dDistDir=$(BIN_DIR)/client installer/client.wxs -out $(BIN_DIR)/client/client.wixobj
	light $(BIN_DIR)/client/client.wixobj -out $(BIN_DIR)/client/bizshuffle-client.msi

clean:
	rm -rf $(BIN_DIR)

msi: