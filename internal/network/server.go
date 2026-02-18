package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync/internal/indexer"
	"sync/internal/protocol"
)

func StartServer(folder string, address string) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()

	log.Println("Listening on", address)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Accept error:", err)
			continue
		}

		go handleConnection(conn, folder)
	}
}

func handleConnection(conn net.Conn, folder string) {
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	// Receive client index
	var clientIndex protocol.IndexMessage
	if err := decoder.Decode(&clientIndex); err != nil {
		log.Println("Decode error:", err)
		return
	}

	// Scan local folder
	localFiles, err := indexer.ScanFolder(folder)
	if err != nil {
		log.Println("Scan error:", err)
		return
	}

	// Send server index
	err = encoder.Encode(protocol.IndexMessage{
		Type:  "INDEX",
		Files: localFiles,
	})
	if err != nil {
		log.Println("Encode error:", err)
		return
	}

	// Wait for request
	var request protocol.RequestMessage
	if err := decoder.Decode(&request); err != nil {
		log.Println("Request decode error:", err)
		return
	}

	for _, hash := range request.Hashes {
		sendFile(conn, folder, localFiles, hash)
	}
}

func sendFile(conn net.Conn, folder string, files []indexer.FileMeta, hash string) {
	for _, f := range files {
		if f.Hash == hash {
			path := filepath.Join(folder, f.RelativePath)

			file, err := os.Open(path)
			if err != nil {
				log.Println("Open file error:", err)
				return
			}
			defer file.Close()

			encoder := json.NewEncoder(conn)

			header := protocol.FileHeaderMessage{
				Type: "FILE",
				Path: f.RelativePath,
				Size: f.Size,
				Hash: f.Hash,
			}

			if err := encoder.Encode(header); err != nil {
				log.Println("Header encode error:", err)
				return
			}

			if _, err := io.Copy(conn, file); err != nil {
				log.Println("File send error:", err)
			}

			return
		}
	}
}
