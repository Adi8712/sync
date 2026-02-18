package main

import (
	"flag"
	"fmt" // Added fmt import

	"os"
	"os/signal"
	"sync/internal/indexer"
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

	// Get or generate DeviceID
	hostname, _ := os.Hostname()
	deviceID := fmt.Sprintf("%s-%s", hostname, *port)

	state := network.NewNetworkState()

	// Initial local index registration for voting
	if local, err := indexer.ScanFolder(*folder); err == nil {
		state.UpdatePeer(deviceID, local)
	}

	logger.Info.Printf("Starting Sync Service [%s] on port %s for folder: %s\n", deviceID, *port, *folder)

	go network.StartListener(*folder, address, deviceID, state)
	go network.StartDiscoveryBroadcaster(*port)

	logger.Info.Println("Starting automatic discovery...")
	go network.DiscoverPeers(deviceID, func(peerAddr string) {
		network.ConnectToPeer(*folder, peerAddr, deviceID, state)
	})

	if *peer != "" {
		network.ConnectToPeer(*folder, *peer, deviceID, state)
	}

	// Interactive Loop
	go func() {
		for {
			fmt.Print("> ")
			var cmd string
			fmt.Scanln(&cmd)
			switch cmd {
			case "status":
				files := state.GetGlobalFiles()
				localFiles, _ := indexer.ScanFolder(*folder)
				localHashes := make(map[string]bool)
				for _, f := range localFiles {
					localHashes[f.Hash] = true
				}

				fmt.Printf("\n--- Global Network View ---\n")
				var missing []indexer.FileMeta
				for i, f := range files {
					status := "OK"
					if !localHashes[f.Hash] {
						status = "MISSING"
						missing = append(missing, f)
					}

					// Check for tie
					_, ok := state.GetConsensusName(f.Hash)
					consensusLabel := "Consensus"
					if !ok {
						consensusLabel = "TIE!"
					}

					fmt.Printf("[%d] %s (%s) - %s [%s]\n", i, f.RelativePath, f.Hash[:8], status, consensusLabel)
				}

				if len(missing) > 0 {
					fmt.Printf("\nYou have %d missing files. Type 'sync all' or 'sync <index>' to download.\n", len(missing))
				}

			case "sync":
				var target string
				fmt.Scanln(&target)
				files := state.GetGlobalFiles()

				if target == "all" {
					localFiles, _ := indexer.ScanFolder(*folder)
					localHashes := make(map[string]bool)
					for _, f := range localFiles {
						localHashes[f.Hash] = true
					}
					for _, f := range files {
						if !localHashes[f.Hash] {
							network.BroadcastFileRequest(f.Hash, f.RelativePath)
						}
					}
				} else {
					var idx int
					if _, err := fmt.Sscanf(target, "%d", &idx); err == nil && idx >= 0 && idx < len(files) {
						f := files[idx]
						network.BroadcastFileRequest(f.Hash, f.RelativePath)
					}
				}

			case "vote":
				var idx int
				var name string
				fmt.Scanln(&idx, &name)
				files := state.GetGlobalFiles()
				if idx >= 0 && idx < len(files) {
					f := files[idx]
					state.SetManualConsensus(f.Hash, name)
					network.BroadcastConsensusVote(f.Hash, name)
					fmt.Printf("Vote cast for %s -> %s\n", f.Hash[:8], name)
				}

			case "rename":
				network.RenameToConsensus(*folder, state)

			case "help":
				fmt.Println("Commands: status, sync [all|idx], vote [idx] [name], rename, exit")
			case "exit":
				os.Exit(0)
			}
		}
	}()

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan
	logger.Info.Println("Shutting down Sync Service...")
}
