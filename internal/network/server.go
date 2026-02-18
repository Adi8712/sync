package network

import (
	"bufio"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"

	"sync/internal/indexer"
	"sync/internal/logger"
	"sync/internal/protocol"
)

func StartServer(folder string, address string) error {
	logger.Info.Println("Starting TCP server on", address)

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()

	logger.Info.Println("Server listening successfully")

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error.Println("Connection accept failed:", err)
			continue
		}

		logger.Info.Println("New connection from", conn.RemoteAddr())
		go handleConnection(conn, folder)
	}
}

func handleConnection(conn net.Conn, folder string) {
	defer func() {
		logger.Info.Println("Closing connection:", conn.RemoteAddr())
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(conn)

	logger.Debug.Println("Waiting for client INDEX message")

	var clientIndex protocol.IndexMessage
	if err := decoder.Decode(&clientIndex); err != nil {
		logger.Error.Println("Failed to decode client index:", err)
		return
	}

	logger.Info.Printf("Received client index: %d files\n", len(clientIndex.Files))

	logger.Debug.Println("Scanning local folder")
	localFiles, err := indexer.ScanFolder(folder)
	if err != nil {
		logger.Error.Println("Folder scan failed:", err)
		return
	}

	logger.Info.Printf("Sending server index: %d files\n", len(localFiles))
	if err := encoder.Encode(protocol.IndexMessage{
		Type:  "INDEX",
		Files: localFiles,
	}); err != nil {
		logger.Error.Println("Failed to send server index:", err)
		return
	}

	logger.Debug.Println("Waiting for file REQUEST")

	var request protocol.RequestMessage
	if err := decoder.Decode(&request); err != nil {
		logger.Error.Println("Failed to decode request:", err)
		return
	}

	logger.Info.Printf("Client requested %d files\n", len(request.Hashes))

	for _, hash := range request.Hashes {
		sendFile(conn, encoder, folder, localFiles, hash)
	}

	logger.Info.Println("All requested files sent successfully")
}

func sendFile(conn net.Conn, encoder *json.Encoder, folder string, files []indexer.FileMeta, hash string) {
	for _, f := range files {
		if f.Hash == hash {
			fullPath := filepath.Join(folder, f.RelativePath)

			logger.Info.Printf("Sending file: %s (%d bytes)\n", f.RelativePath, f.Size)

			file, err := os.Open(fullPath)
			if err != nil {
				logger.Error.Println("Failed to open file:", err)
				return
			}
			defer file.Close()

			header := protocol.FileHeaderMessage{
				Type: "FILE",
				Path: f.RelativePath,
				Size: f.Size,
				Hash: f.Hash,
			}

			if err := encoder.Encode(header); err != nil {
				logger.Error.Println("Failed to send file header:", err)
				return
			}

			written, err := io.Copy(conn, file)
			if err != nil {
				logger.Error.Println("File transfer failed:", err)
				return
			}

			logger.Debug.Printf("File sent successfully (%d bytes written)\n", written)
			return
		}
	}

	logger.Error.Println("Requested hash not found:", hash)
}
