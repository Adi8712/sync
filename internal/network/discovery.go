package network

import (
	"fmt"
	"net"
	"strings"
	"sync/internal/logger"
	"time"
)

const (
	discoveryPort = 9999
	discoveryMsg  = "SYNC_PEER_DISCOVERY"
)

func StartDiscoveryBroadcaster(port string) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("255.255.255.255:%d", discoveryPort))
	if err != nil {
		logger.Error.Println("Discovery broadcaster resolve failed:", err)
		return
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		logger.Error.Println("Discovery broadcaster dial failed:", err)
		return
	}
	defer conn.Close()

	payload := fmt.Sprintf("%s:%s", discoveryMsg, port)

	for {
		_, err := conn.Write([]byte(payload))
		if err != nil {
			logger.Error.Println("Discovery broadcast failed:", err)
		}
		time.Sleep(5 * time.Second)
	}
}

func DiscoverPeers(myDeviceID string, onPeerFound func(address string)) {
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf(":%d", discoveryPort))
	if err != nil {
		logger.Error.Println("Discovery listener resolve failed:", err)
		return
	}

	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		logger.Error.Println("Discovery listener listen failed:", err)
		return
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	discovered := make(map[string]bool)

	for {
		n, remoteAddr, err := conn.ReadFromUDP(buffer)
		if err != nil {
			logger.Error.Println("Discovery read error:", err)
			continue
		}

		msg := string(buffer[:n])
		if strings.HasPrefix(msg, discoveryMsg) {
			parts := strings.Split(msg, ":")
			if len(parts) == 2 {
				peerPort := parts[1]
				peerAddr := fmt.Sprintf("%s:%s", remoteAddr.IP.String(), peerPort)

				if !isLocalIP(remoteAddr.IP) && !discovered[peerAddr] {
					discovered[peerAddr] = true
					onPeerFound(peerAddr)
				}
			}
		}
	}
}

func isLocalIP(ip net.IP) bool {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, i := range ifaces {
		addrs, err := i.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var localIP net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				localIP = v.IP
			case *net.IPAddr:
				localIP = v.IP
			}
			if localIP.Equal(ip) {
				return true
			}
		}
	}
	return false
}
