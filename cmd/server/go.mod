module github.com/michael4d45/bizshuffle/cmd/server

go 1.26.0

require (
	github.com/michael4d45/bizshuffle/clienthost v0.0.0
	github.com/michael4d45/bizshuffle/serverhost v0.0.0
)

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/michael4d45/bizshuffle/obslog v0.0.0 // indirect
	github.com/michael4d45/bizshuffle/protocol v0.0.0 // indirect
	github.com/michael4d45/bizshuffle/savestate v0.0.0 // indirect
)

replace (
	github.com/michael4d45/bizshuffle/assets => ../../assets
	github.com/michael4d45/bizshuffle/clienthost => ../../clienthost
	github.com/michael4d45/bizshuffle/obslog => ../../obslog
	github.com/michael4d45/bizshuffle/protocol => ../../protocol
	github.com/michael4d45/bizshuffle/savestate => ../../savestate
	github.com/michael4d45/bizshuffle/serverhost => ../../serverhost
)
