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
	syncengine "sync/internal/sync"
)

func StartClient(folder string, address string) error {
	logger.Info.Println("Connecting to server:", address)

	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer func() {
		logger.Info.Println("Closing client connection")
		conn.Close()
	}()

	reader := bufio.NewReader(conn)
	decoder := json.NewDecoder(reader)
	encoder := json.NewEncoder(conn)

	logger.Info.Println("Connected successfully")

	logger.Debug.Println("Scanning local folder")
	localFiles, err := indexer.ScanFolder(folder)
	if err != nil {
		return err
	}

	logger.Info.Printf("Local index contains %d files\n", len(localFiles))

	logger.Debug.Println("Sending local INDEX")
	if err := encoder.Encode(protocol.IndexMessage{
		Type:  "INDEX",
		Files: localFiles,
	}); err != nil {
		return err
	}

	logger.Debug.Println("Waiting for server INDEX")
	var serverIndex protocol.IndexMessage
	if err := decoder.Decode(&serverIndex); err != nil {
		return err
	}

	logger.Info.Printf("Received server index: %d files\n", len(serverIndex.Files))

	diff := syncengine.Compare(localFiles, serverIndex.Files)

	logger.Info.Printf("Missing files to fetch: %d\n", len(diff.MissingInA))
	logger.Info.Printf("Conflicts detected: %d\n", len(diff.Conflicts))

	var hashes []string
	for _, f := range diff.MissingInA {
		hashes = append(hashes, f.Hash)
	}

	logger.Debug.Println("Sending REQUEST message")
	if err := encoder.Encode(protocol.RequestMessage{
		Type:   "REQUEST",
		Hashes: hashes,
	}); err != nil {
		return err
	}

	for range hashes {
		var header protocol.FileHeaderMessage
		if err := decoder.Decode(&header); err != nil {
			return err
		}

		logger.Info.Printf("Receiving file: %s (%d bytes)\n", header.Path, header.Size)

		fullPath := filepath.Join(folder, header.Path)
		os.MkdirAll(filepath.Dir(fullPath), os.ModePerm)

		file, err := os.Create(fullPath)
		if err != nil {
			return err
		}

		written, err := io.CopyN(file, reader, header.Size)
		file.Close()
		if err != nil {
			return err
		}

		logger.Debug.Printf("File written successfully (%d bytes)\n", written)
	}

	logger.Info.Println("Synchronization complete")
	return nil
}
