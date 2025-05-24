package main

import (
	"log"
	"github.com/DangerosoDavo/udp_network"
)

func main() {
	server, err := udp_network.NewServer(":8080")
	if err != nil {
		log.Fatal(err)
	}
	
	server.Start()
	defer server.Stop()
	
	log.Println("UDP server running on :8080")
	
	// Keep running
	select {}
}