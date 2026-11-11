module example-game

go 1.27.4

require github.com/DangerosoDavo/udp_network v0.0.0

require (
	github.com/gorilla/websocket v1.5.3 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.50.0 // indirect
)

replace github.com/DangerosoDavo/udp_network => ../
