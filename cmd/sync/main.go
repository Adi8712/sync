package main

import (
	"flag"

	"sync/internal/logger"
	"sync/internal/network"
)

func main() {
	logger.Init()

	mode := flag.String("mode", "", "serve or connect")
	folder := flag.String("folder", "", "Folder path")
	address := flag.String("addr", ":9000", "Address")

	flag.Parse()

	logger.Info.Println("Sync application starting")
	logger.Info.Println("Mode:", *mode)
	logger.Info.Println("Folder:", *folder)
	logger.Info.Println("Address:", *address)

	switch *mode {
	case "serve":
		if err := network.StartServer(*folder, *address); err != nil {
			logger.Error.Println("Server terminated with error:", err)
		}
	case "connect":
		if err := network.StartClient(*folder, *address); err != nil {
			logger.Error.Println("Client terminated with error:", err)
		}
	default:
		logger.Error.Println("Invalid mode. Use 'serve' or 'connect'")
	}
}
