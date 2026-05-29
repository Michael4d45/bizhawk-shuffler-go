module github.com/michael4d45/bizshuffle/clienthost

go 1.26.0

require (
	github.com/go-vgo/robotgo v0.110.8
	github.com/gorilla/websocket v1.5.3
	github.com/michael4d45/bizshuffle/assets v0.0.0
	github.com/michael4d45/bizshuffle/protocol v0.0.0
	github.com/michael4d45/bizshuffle/savestate v0.0.0
	golang.org/x/sys v0.43.0
)

require (
	github.com/dblohm7/wingoes v0.0.0-20240820181039-f2b84150679e // indirect
	github.com/ebitengine/purego v0.9.1 // indirect
	github.com/gen2brain/shm v0.1.1 // indirect
	github.com/go-ole/go-ole v1.3.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/jezek/xgb v1.1.1 // indirect
	github.com/klauspost/compress v1.18.0 // indirect
	github.com/lufia/plan9stats v0.0.0-20250317134145-8bc96cf8fc35 // indirect
	github.com/otiai10/gosseract v2.2.1+incompatible // indirect
	github.com/otiai10/mint v1.6.3 // indirect
	github.com/power-devops/perfstat v0.0.0-20240221224432-82ca36839d55 // indirect
	github.com/robotn/xgb v0.10.0 // indirect
	github.com/robotn/xgbutil v0.10.0 // indirect
	github.com/shirou/gopsutil/v4 v4.26.1 // indirect
	github.com/tailscale/win v0.0.0-20250213223159-5992cb43ca35 // indirect
	github.com/tklauser/go-sysconf v0.3.16 // indirect
	github.com/tklauser/numcpus v0.11.0 // indirect
	github.com/vcaesar/gops v0.41.0 // indirect
	github.com/vcaesar/imgo v0.41.0 // indirect
	github.com/vcaesar/keycode v0.10.1 // indirect
	github.com/vcaesar/screenshot v0.11.1 // indirect
	github.com/vcaesar/tt v0.20.1 // indirect
	github.com/yusufpapurcu/wmi v1.2.4 // indirect
	golang.org/x/exp v0.0.0-20250620022241-b7579e27df2b // indirect
	golang.org/x/image v0.38.0 // indirect
	golang.org/x/net v0.53.0 // indirect
)

replace (
	github.com/michael4d45/bizshuffle/assets => ../assets
	github.com/michael4d45/bizshuffle/protocol => ../protocol
	github.com/michael4d45/bizshuffle/savestate => ../savestate
)
