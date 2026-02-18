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

	logger.Info.Printf("Sending local index (%d files)\n", len(localFiles))
	idxMsg, _ := json.Marshal(IndexMessage{
		Type:  "INDEX",
		Files: localFiles,
	})
	conn.Write(append(idxMsg, '\n'))

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		logger.Error.Println("Failed reading remote index:", err)
		return
	}

	var remoteIndex IndexMessage
	if err := json.Unmarshal(line, &remoteIndex); err != nil {
		logger.Error.Println("Failed unmarshaling remote index:", err)
		return
	}

	logger.Info.Printf("Received remote index (%d files)\n", len(remoteIndex.Files))

	// Phase 2 — Sync Analysis
	diff := indexer.Compare(localFiles, remoteIndex.Files)

	// Display Status
	if len(diff.MissingInA) > 0 {
		logger.Info.Println("Files to receive from peer:")
		for _, f := range diff.MissingInA {
			logger.Info.Printf("  + %s (%d bytes)\n", f.RelativePath, f.Size)
		}
	}
	if len(diff.MissingInB) > 0 {
		logger.Info.Println("Files to send to peer:")
		for _, f := range diff.MissingInB {
			logger.Info.Printf("  - %s (%d bytes)\n", f.RelativePath, f.Size)
		}
	}

	// Phase 3 — Conflict Resolution (Incoming Truth)
	// If a file exists on both but hashes differ, we rename local and receive remote.
	for _, c := range diff.Conflicts {
		localPath := filepath.Join(folder, c.Path)
		conflictName := localPath + ".conflict"

		logger.Info.Printf("Conflict detected: %s. Renaming local version to %s and accepting remote.\n", c.Path, filepath.Base(conflictName))
		if err := os.Rename(localPath, conflictName); err != nil {
			logger.Error.Printf("Failed to rename conflicting file %s: %v\n", c.Path, err)
			continue
		}

		// After renaming local, the remote file is effectively "MissingInA"
		diff.MissingInA = append(diff.MissingInA, c.B)
	}

	if len(diff.MissingInA) == 0 && len(diff.MissingInB) == 0 && len(diff.Conflicts) == 0 {
		logger.Info.Println("All files are up to date.")
		return
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
