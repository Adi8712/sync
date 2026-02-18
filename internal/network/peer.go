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

var (
	connMu sync.Mutex
	conns  = make(map[string]net.Conn)
)

func Start(fld, addr, id string, st *NetworkState) {
	c, _ := GetCert()
	l, _ := tls.Listen("tcp", addr, TLSConfig(c))
	for {
		conn, _ := l.Accept()
		go handle(conn, fld, id, st)
	}
}

func Connect(fld, addr, id string, st *NetworkState) {
	c, _ := GetCert()
	conn, _ := tls.Dial("tcp", addr, TLSConfig(c))
	go handle(conn, fld, id, st)
}

func handle(conn net.Conn, fld, id string, st *NetworkState) {
	defer conn.Close()
	idx, _ := indexer.ScanFolder(fld)
	b, _ := json.Marshal(IndexMsg{"INDEX", id, idx})
	conn.Write(append(b, '\n'))

	r := bufio.NewReader(conn)
	for {
		ln, err := r.ReadBytes('\n')
		if err != nil {
			return
		}

		var m map[string]any
		json.Unmarshal(ln, &m)
		switch m["t"] {
		case "INDEX":
			var msg IndexMsg
			json.Unmarshal(ln, &msg)
			st.Update(msg.Device, msg.Files)
			connMu.Lock()
			conns[msg.Device] = conn
			connMu.Unlock()
		case "VOTE":
			var msg VoteMsg
			json.Unmarshal(ln, &msg)
			st.SetVote(msg.Hash, msg.Name)
		case "REQ":
			var msg ReqMsg
			json.Unmarshal(ln, &msg)
			files, _ := indexer.ScanFolder(fld)
			for _, f := range files {
				if f.Hash == msg.Hash {
					sendFile(conn, fld, f)
					break
				}
			}
		case "FILE":
			var msg FileHeader
			json.Unmarshal(ln, &msg)
			recvFile(r, msg, fld)
		}
	}
}

func sendFile(c net.Conn, fld string, f indexer.FileMeta) {
	h, _ := json.Marshal(FileHeader{"FILE", f.RelativePath, f.Size, f.Hash})
	c.Write(append(h, '\n'))
	fl, _ := os.Open(filepath.Join(fld, f.RelativePath))
	defer fl.Close()
	io.Copy(c, fl)
}

func recvFile(r *bufio.Reader, h FileHeader, fld string) {
	pth := filepath.Join(fld, h.Path)
	os.MkdirAll(filepath.Dir(pth), os.ModePerm)
	fl, _ := os.Create(pth)
	defer fl.Close()

	hs := sha256.New()
	mw := io.MultiWriter(fl, hs)
	io.CopyN(mw, r, h.Size)

	if hex.EncodeToString(hs.Sum(nil)) != h.Hash {
		os.Remove(pth)
		logger.Err("Hash fail: %s", h.Path)
	} else {
		logger.Done("Got: %s", h.Path)
	}
}

func BroadcastReq(h, p string) {
	b, _ := json.Marshal(ReqMsg{"REQ", h, p})
	connMu.Lock()
	for _, c := range conns {
		c.Write(append(b, '\n'))
	}
	connMu.Unlock()
}

func BroadcastIdx(fld, id string) {
	idx, _ := indexer.ScanFolder(fld)
	b, _ := json.Marshal(IndexMsg{"INDEX", id, idx})
	connMu.Lock()
	for _, c := range conns {
		c.Write(append(b, '\n'))
	}
	connMu.Unlock()
}

func BroadcastVote(h, n string) {
	b, _ := json.Marshal(VoteMsg{"VOTE", h, n})
	connMu.Lock()
	for _, c := range conns {
		c.Write(append(b, '\n'))
	}
	connMu.Unlock()
}
