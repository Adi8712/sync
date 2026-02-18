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

func StartListener(folder string, address string, myDeviceID string, state *NetworkState) error {
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
		go handleConnection(conn, folder, myDeviceID, state)
	}
}

func ConnectToPeer(folder string, address string, myDeviceID string, state *NetworkState) error {
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

	go handleConnection(conn, folder, myDeviceID, state)
	return nil
}

var (
	connMu sync.Mutex
	conns  = make(map[string]net.Conn) // DeviceID -> connection
)

func registerConn(id string, conn net.Conn) {
	connMu.Lock()
	defer connMu.Unlock()
	conns[id] = conn
}

func BroadcastFileRequest(hash, path string) {
	connMu.Lock()
	defer connMu.Unlock()

	req, _ := json.Marshal(FileRequestMessage{
		Type: "FILE_REQUEST",
		Hash: hash,
		Path: path,
	})

	for id, conn := range conns {
		_, err := conn.Write(append(req, '\n'))
		if err != nil {
			logger.Error.Printf("Failed to request file from %s: %v\n", id, err)
		}
	}
}

func RenameToConsensus(folder string, state *NetworkState) {
	localFiles, _ := indexer.ScanFolder(folder)
	for _, f := range localFiles {
		winner, ok := state.GetConsensusName(f.Hash)
		if ok && winner != f.RelativePath {
			logger.Info.Printf("Renaming local %s to consensus name: %s\n", f.RelativePath, winner)
			if err := indexer.RenameFile(folder, f.RelativePath, winner); err != nil {
				logger.Error.Printf("Rename failed for %s -> %s: %v\n", f.RelativePath, winner, err)
			}
		}
	}
}

func BroadcastConsensusVote(hash, name string) {
	connMu.Lock()
	defer connMu.Unlock()

	vote, _ := json.Marshal(ConsensusVoteMessage{
		Type: "CONSENSUS_VOTE",
		Hash: hash,
		Name: name,
	})

	for _, conn := range conns {
		conn.Write(append(vote, '\n'))
	}
}

func handleConnection(conn net.Conn, folder string, myDeviceID string, state *NetworkState) {
	defer conn.Close()

	// Immediately send our index
	localFiles, err := indexer.ScanFolder(folder)
	if err == nil {
		idxMsg, _ := json.Marshal(IndexMessage{
			Type:     "INDEX",
			DeviceID: myDeviceID,
			Files:    localFiles,
		})
		conn.Write(append(idxMsg, '\n'))
	}

	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logger.Error.Println("Connection error:", err)
			}
			return
		}

		var raw map[string]interface{}
		if err := json.Unmarshal(line, &raw); err != nil {
			continue
		}

		switch raw["type"] {
		case "INDEX":
			var msg IndexMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			state.UpdatePeer(msg.DeviceID, msg.Files)
			registerConn(msg.DeviceID, conn)
			logger.Info.Printf("Updated state for peer: %s (%d files)\n", msg.DeviceID, len(msg.Files))

		case "CONSENSUS_VOTE":
			var msg ConsensusVoteMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			state.SetManualConsensus(msg.Hash, msg.Name)
			logger.Info.Printf("Consensus vote received for %s: %s\n", msg.Hash[:8], msg.Name)

		case "FILE_REQUEST":
			var msg FileRequestMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			logger.Info.Printf("Peer requested file: %s (%s)\n", msg.Path, msg.Hash[:8])

			// Find file by hash locally
			localFiles, _ := indexer.ScanFolder(folder)
			var found *indexer.FileMeta
			for _, f := range localFiles {
				if f.Hash == msg.Hash {
					found = &f
					break
				}
			}

			if found != nil {
				sendMissingFiles(conn, folder, []indexer.FileMeta{*found})
			}

		case "FILE":
			// We need to pass the reader to receiveFiles because it has the binary data
			// However, our loop already read the JSON line. receiveFiles needs to know what it just read.
			var msg FileHeaderMessage
			if err := json.Unmarshal(line, &msg); err != nil {
				continue
			}
			receiveOneFile(reader, msg, folder)
		}
	}
}

func receiveOneFile(reader *bufio.Reader, msg FileHeaderMessage, folder string) {
	fullPath := filepath.Join(folder, msg.Path)
	os.MkdirAll(filepath.Dir(fullPath), os.ModePerm)

	logger.Info.Printf("Receiving file: %s (%d bytes)\n", msg.Path, msg.Size)

	file, err := os.Create(fullPath)
	if err != nil {
		logger.Error.Println("Create failed:", err)
		io.CopyN(io.Discard, reader, msg.Size)
		return
	}
	defer file.Close()

	hasher := sha256.New()
	multi := io.MultiWriter(file, hasher)

	_, err = io.CopyN(multi, reader, msg.Size)
	if err != nil {
		logger.Error.Println("Receive file copy failed:", err)
		return
	}

	actualHash := hex.EncodeToString(hasher.Sum(nil))
	if actualHash != msg.Hash {
		logger.Error.Printf("Hash mismatch for %s! Expected: %s, Got: %s\n", msg.Path, msg.Hash, actualHash)
		os.Remove(fullPath)
	} else {
		logger.Info.Printf("Successfully received and verified: %s\n", msg.Path)
	}
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
