package network

import (
	"fmt"
	"net"
	"sync/internal/logger"
	"time"
)

func Broadcast(port string) {
	addr, _ := net.ResolveUDPAddr("udp", "255.255.255.255:9999")
	conn, _ := net.DialUDP("udp", nil, addr)
	defer conn.Close()

	for {
		conn.Write([]byte(port))
		time.Sleep(5 * time.Second)
	}
}

func Discover(myID string, onPeer func(string)) {
	addr, _ := net.ResolveUDPAddr("udp", ":9999")
	conn, _ := net.ListenUDP("udp", addr)
	defer conn.Close()

	seen := make(map[string]bool)
	for {
		buf := make([]byte, 1024)
		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}

		addr := fmt.Sprintf("%s:%s", raddr.IP, string(buf[:n]))
		if !seen[addr] {
			seen[addr] = true
			logger.Info("Discovered peer: %s", addr)
			onPeer(addr)
		}
	}
}
