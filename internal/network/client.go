package network

import (
	"encoding/json"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"sync/internal/indexer"
	syncengine "sync/internal/sync"
	"sync/internal/protocol"
)

func StartClient(folder string, address string) error {
	conn, err := net.Dial("tcp", address)
	if err != nil {
		return err
	}
	defer conn.Close()

	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	// Scan local
	localFiles, err := indexer.ScanFolder(folder)
	if err != nil {
		return err
	}

	// Send index
	err = encoder.Encode(protocol.IndexMessage{
		Type:  "INDEX",
		Files: localFiles,
	})
	if err != nil {
		return err
	}

	// Receive server index
	var serverIndex protocol.IndexMessage
	if err := decoder.Decode(&serverIndex); err != nil {
		return err
	}

	// Compare
	diff := syncengine.Compare(localFiles, serverIndex.Files)

	// Request missing files
	var hashes []string
	for _, f := range diff.MissingInA {
		hashes = append(hashes, f.Hash)
	}

	err = encoder.Encode(protocol.RequestMessage{
		Type:   "REQUEST",
		Hashes: hashes,
	})
	if err != nil {
		return err
	}

	// Receive files
	for range hashes {
		var header protocol.FileHeaderMessage
		if err := decoder.Decode(&header); err != nil {
			return err
		}

		path := filepath.Join(folder, header.Path)
		os.MkdirAll(filepath.Dir(path), os.ModePerm)

		file, err := os.Create(path)
		if err != nil {
			return err
		}

		_, err = io.CopyN(file, conn, header.Size)
		file.Close()
		if err != nil {
			return err
		}
	}

	log.Println("Sync complete")
	return nil
}
