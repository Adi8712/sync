package network

import (
	"bufio"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"sync/internal/indexer"
	"sync/internal/logger"
)

func StartListener(folder string, address string) error {
	cert, err := GenerateSelfSignedCert()
	if err != nil {
		return err
	}
	tlsConfig := GetTLSConfig(cert)

	logger.Info.Println("Starting secure peer listener on", address)

	listener, err := tls.Listen("tcp", address, tlsConfig)
	if err != nil {
		return err
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Error.Println("Accept failed:", err)
			continue
		}

		logger.Info.Println("Incoming secure connection from", conn.RemoteAddr())
		go handleConnection(conn, folder)
	}
}

func ConnectToPeer(folder string, address string) error {
	cert, err := GenerateSelfSignedCert()
	if err != nil {
		return err
	}
	tlsConfig := GetTLSConfig(cert)

	logger.Info.Println("Connecting securely to peer:", address)

	conn, err := tls.Dial("tcp", address, tlsConfig)
	if err != nil {
		return err
	}

	go handleConnection(conn, folder)
	return nil
}

func handleConnection(conn net.Conn, folder string) {
	defer conn.Close()

	logger.Info.Println("Connection established with", conn.RemoteAddr())

	// Phase 1 — Exchange Index
	localFiles, err := indexer.ScanFolder(folder)
	if err != nil {
		logger.Error.Println("Scan failed:", err)
		return
	}

	logger.Info.Printf("Sending INDEX (%d files)\n", len(localFiles))
	idxMsg, _ := json.Marshal(IndexMessage{
		Type:  "INDEX",
		Files: localFiles,
	})
	conn.Write(append(idxMsg, '\n'))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		logger.Error.Println("Failed reading INDEX:", err)
		return
	}

	var remoteIndex IndexMessage
	if err := json.Unmarshal(line, &remoteIndex); err != nil {
		logger.Error.Println("Failed unmarshaling INDEX:", err)
		return
	}

	logger.Info.Printf("Received INDEX (%d files)\n", len(remoteIndex.Files))

	// Phase 2 — Diff
	diff := indexer.Compare(localFiles, remoteIndex.Files)

	// Phase 3 — Conflict Resolution
	for _, c := range diff.Conflicts {
		conflictPath := filepath.Join(folder, c.Path)
		newName := conflictPath + ".conflict." + c.A.Hash

		logger.Info.Println("Conflict detected. Renaming:", conflictPath)
		os.Rename(conflictPath, newName)
	}

	// Recompute after conflict rename
	if len(diff.Conflicts) > 0 {
		localFiles, _ = indexer.ScanFolder(folder)
		diff = indexer.Compare(localFiles, remoteIndex.Files)
	}

	var wg sync.WaitGroup
	wg.Add(2)

	// Sender
	go func() {
		defer wg.Done()
		sendMissingFiles(conn, folder, diff.MissingInB)
		doneMsg, _ := json.Marshal(DoneMessage{Type: "DONE"})
		conn.Write(append(doneMsg, '\n'))
		logger.Info.Println("Finished sending files")
	}()

	// Receiver
	go func() {
		defer wg.Done()
		receiveFiles(reader, conn, folder)
		logger.Info.Println("Finished receiving files")
	}()

	wg.Wait()
	logger.Info.Println("Peer synchronization complete")
}

func sendMissingFiles(conn net.Conn, folder string, files []indexer.FileMeta) {
	for _, f := range files {
		fullPath := filepath.Join(folder, f.RelativePath)

		logger.Info.Printf("Sending file: %s (%d bytes)\n", f.RelativePath, f.Size)

		file, err := os.Open(fullPath)
		if err != nil {
			logger.Error.Println("Open failed:", err)
			continue
		}

		header, _ := json.Marshal(FileHeaderMessage{
			Type: "FILE",
			Path: f.RelativePath,
			Size: f.Size,
			Hash: f.Hash,
		})
		conn.Write(append(header, '\n'))

		// Send binary data
		_, err = io.Copy(conn, file)
		if err != nil {
			logger.Error.Println("File copy failed:", err)
		}
		file.Close()
	}
}

func receiveFiles(reader *bufio.Reader, conn net.Conn, folder string) {
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logger.Error.Println("Receive read error:", err)
			}
			return
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(line, &raw); err != nil {
			logger.Error.Println("Receive unmarshal error:", err, "Line:", string(line))
			continue
		}

		switch raw["type"] {
		case "FILE":
			path := raw["path"].(string)
			size := int64(raw["size"].(float64))
			expectedHash := raw["hash"].(string)

			fullPath := filepath.Join(folder, path)
			os.MkdirAll(filepath.Dir(fullPath), os.ModePerm)

			logger.Info.Printf("Receiving file: %s (%d bytes)\n", path, size)

			file, err := os.Create(fullPath)
			if err != nil {
				logger.Error.Println("Create failed:", err)
				// We still need to drain the connection
				io.CopyN(io.Discard, reader, size)
				continue
			}

			// Wrap reader to compute hash while receiving
			hasher := sha256.New()
			multi := io.MultiWriter(file, hasher)

			_, err = io.CopyN(multi, reader, size)
			file.Close()

			if err != nil {
				logger.Error.Println("Receive file copy failed:", err)
				continue
			}

			actualHash := hex.EncodeToString(hasher.Sum(nil))
			if actualHash != expectedHash {
				logger.Error.Printf("Hash mismatch for %s! Expected: %s, Got: %s\n", path, expectedHash, actualHash)
				os.Remove(fullPath) // Remove corrupted file
			} else {
				logger.Info.Printf("Successfully received and verified: %s\n", path)
			}

		case "DONE":
			logger.Info.Println("Received DONE signal")
			return
		}
	}
}
