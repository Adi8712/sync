package main

import (
	"flag"
	"time"

	"os"
	"os/signal"
	"sync/internal/logger"
	"sync/internal/network"
	"syscall"
)

func main() {
	logger.Init()

	folder := flag.String("folder", "", "Folder path")
	port := flag.String("port", "9000", "Listening port")
	peer := flag.String("peer", "", "Peer address")

	flag.Parse()

	if *folder == "" {
		logger.Error.Fatal("Folder path is required")
	}

	address := ":" + *port

	logger.Info.Printf("Starting Sync Service on port %s for folder: %s\n", *port, *folder)

	go network.StartListener(*folder, address)
	go network.StartDiscoveryBroadcaster(*port)

	if *peer != "" {
		time.Sleep(1 * time.Second)
		network.ConnectToPeer(*folder, *peer)
	} else {
		logger.Info.Println("No peer specified. Starting automatic discovery...")
		go network.DiscoverPeers(func(peerAddr string) {
			network.ConnectToPeer(*folder, peerAddr)
		})
	}

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	logger.Info.Println("Shutting down Sync Service...")
}
