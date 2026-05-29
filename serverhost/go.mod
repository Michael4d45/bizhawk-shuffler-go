module github.com/michael4d45/bizshuffle/serverhost

go 1.26.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/michael4d45/bizshuffle/obslog v0.0.0
	github.com/michael4d45/bizshuffle/protocol v0.0.0
	github.com/michael4d45/bizshuffle/savestate v0.0.0
)

require github.com/klauspost/compress v1.18.0 // indirect

replace (
	github.com/michael4d45/bizshuffle/obslog => ../obslog
	github.com/michael4d45/bizshuffle/protocol => ../protocol
	github.com/michael4d45/bizshuffle/savestate => ../savestate
)
