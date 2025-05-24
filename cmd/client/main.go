package main

import (
	"fmt"
	"log"
	"os"
	"udp_network"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: client <server:port>")
		os.Exit(1)
	}
	
	client, err := udp_network.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	
	client.Start()
	defer client.Stop()
	
	if err := client.Connect(os.Args[1]); err != nil {
		log.Fatal(err)
	}
	
	client.RunInteractive()
}