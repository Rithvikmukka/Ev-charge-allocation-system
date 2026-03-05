package internal

import (
	"net"
	"strconv"
	"strings"
)

// isSelfAddr checks if the given address belongs to this node
func isSelfAddr(addr string, port int) bool {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		// If parsing fails, try simple comparison
		return addr == net.JoinHostPort("localhost", strconv.Itoa(port)) ||
			addr == net.JoinHostPort("0.0.0.0", strconv.Itoa(port))
	}
	
	portNum, err := strconv.Atoi(p)
	if err != nil {
		return false
	}
	
	// Check if port matches and host is local
	if portNum != port {
		return false
	}
	
	// Check for localhost variants
	switch host {
	case "localhost", "0.0.0.0", "::1", "127.0.0.1":
		return true
	}
	
	// Check if it matches our actual IP
	return host == getLocalHost()
}

// getLocalHost returns the local non-loopback IP address
func getLocalHost() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "localhost"
	}
	
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	
	return "localhost"
}

// parseAddr extracts host and port from address string
func parseAddr(addr string) (host string, port int, err error) {
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	
	port, err = strconv.Atoi(p)
	if err != nil {
		return "", 0, err
	}
	
	return host, port, nil
}

// normalizeAddress ensures address is in host:port format
func normalizeAddress(addr string, defaultPort int) string {
	if !strings.Contains(addr, ":") {
		return net.JoinHostPort(addr, strconv.Itoa(defaultPort))
	}
	return addr
}
