package main

import (
	"flag"
	"log"
	"sync/internal/network"
)

func main() {
	mode := flag.String("mode", "", "serve or connect")
	folder := flag.String("folder", "", "Folder path")
	address := flag.String("addr", "localhost:9000", "Address")

	flag.Parse()

	if *mode == "" || *folder == "" {
		log.Fatal("Provide --mode and --folder")
	}

	switch *mode {
	case "serve":
		log.Fatal(network.StartServer(*folder, *address))
	case "connect":
		log.Fatal(network.StartClient(*folder, *address))
	default:
		log.Fatal("Unknown mode")
	}
}
