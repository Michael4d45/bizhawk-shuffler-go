module github.com/michael4d45/bizshuffle/serverhost

go 1.26.0

require (
	github.com/gorilla/websocket v1.5.3
	github.com/michael4d45/bizshuffle/protocol v0.0.0
)

replace github.com/michael4d45/bizshuffle/protocol => ../protocol
