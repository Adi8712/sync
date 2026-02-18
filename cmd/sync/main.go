package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
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
		scanner := bufio.NewScanner(os.Stdin)
		for {
			fmt.Print("> ")
			if !scanner.Scan() {
				break
			}
			input := scanner.Text()
			parts := strings.Fields(input)
			if len(parts) == 0 {
				continue
			}

			cmd := parts[0]
			switch cmd {
			case "status":
				// Re-scan local and broadcast updated state to refresh network
				local, err := indexer.ScanFolder(*folder)
				if err == nil {
					state.UpdatePeer(deviceID, local)
					network.BroadcastIndex(*folder, deviceID)
				}

				files := state.GetGlobalFiles() // Sorted alphabetically
				localHashes := make(map[string]bool)
				for _, f := range local {
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

					collision := ""
					if (i > 0 && files[i-1].RelativePath == f.RelativePath) ||
						(i < len(files)-1 && files[i+1].RelativePath == f.RelativePath) {
						collision = " [COLLISION]"
					}

					_, ok := state.GetConsensusName(f.Hash)
					consensusLabel := "Consensus"
					if !ok {
						consensusLabel = "TIE!"
					}

					fmt.Printf("[%d] %s (%s) - %s [%s]%s\n", i, f.RelativePath, f.Hash[:8], status, consensusLabel, collision)
				}

				if len(missing) > 0 {
					fmt.Printf("\nYou have %d missing files. Type 'sync all' or 'sync <index>' to download.\n", len(missing))
				}

			case "sync":
				if len(parts) < 2 {
					fmt.Println("Usage: sync [all|idx]")
					continue
				}
				files := state.GetGlobalFiles()
				if parts[1] == "all" {
					local, _ := indexer.ScanFolder(*folder)
					localHashes := make(map[string]bool)
					for _, f := range local {
						localHashes[f.Hash] = true
					}
					for _, f := range files {
						if !localHashes[f.Hash] {
							network.BroadcastFileRequest(f.Hash, f.RelativePath)
						}
					}
				} else if idx, err := strconv.Atoi(parts[1]); err == nil && idx >= 0 && idx < len(files) {
					f := files[idx]
					network.BroadcastFileRequest(f.Hash, f.RelativePath)
				}

			case "rename":
				if len(parts) < 3 {
					fmt.Println("Usage: rename [idx] [new_name]")
					continue
				}
				idx, _ := strconv.Atoi(parts[1])
				newName := parts[2]
				files := state.GetGlobalFiles()
				if idx >= 0 && idx < len(files) {
					f := files[idx]
					oldPath := filepath.Join(*folder, f.RelativePath)
					newPath := filepath.Join(*folder, newName)

					os.MkdirAll(filepath.Dir(newPath), os.ModePerm)
					err := os.Rename(oldPath, newPath)
					if err != nil {
						fmt.Printf("Local rename failed: %v\n", err)
					} else {
						state.SetManualConsensus(f.Hash, newName)
						network.BroadcastConsensusVote(f.Hash, newName)
						fmt.Printf("Renamed locally and broadcast vote for %s -> %s\n", f.Hash[:8], newName)
						// Trigger status logic
						fmt.Println("Refreshing status...")
						parts = []string{"status"}
						goto runCommand
					}
				}

			case "vote":
				if len(parts) < 3 {
					fmt.Println("Usage: vote [idx] [new_name]")
					continue
				}
				idx, _ := strconv.Atoi(parts[1])
				name := parts[2]
				files := state.GetGlobalFiles()
				if idx >= 0 && idx < len(files) {
					f := files[idx]
					state.SetManualConsensus(f.Hash, name)
					network.BroadcastConsensusVote(f.Hash, name)
					fmt.Printf("Vote cast for %s -> %s\n", f.Hash[:8], name)
				}

			case "help":
				fmt.Println("Commands: status, sync [all|idx], rename [idx] [name], vote [idx] [name], exit")
			case "exit":
				os.Exit(0)
			}
			continue

		runCommand:
			// Poor man's goto to re-run case switch.
			// In a real app we'd wrap the switch in a function.
			// Let's just avoid the goto and print a message instead.
			fmt.Println("Command complete. Run 'status' to see results.")
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan
	logger.Info.Println("Shutting down Sync Service...")
}
