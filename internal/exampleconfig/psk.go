// Package exampleconfig supplies explicit credentials for executable examples.
// It is not part of the public networking API.
package exampleconfig

import (
	"encoding/base64"
	"log"
	"os"
)

// PSK reads a base64 secret shared out-of-band by the example client/server.
// There is deliberately no built-in default key.
func PSK() []byte {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("UDP_NETWORK_PSK"))
	if err != nil || len(key) < 32 {
		log.Fatal("set UDP_NETWORK_PSK to a base64-encoded random secret of at least 32 bytes; see MAINTENANCE.md")
	}
	return key
}
