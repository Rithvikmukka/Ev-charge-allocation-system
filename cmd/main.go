package main

import (
	"flag"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	"distributed-ev/internal"
)

func getLocalIP() string {
	addrs, _ := net.InterfaceAddrs()
	for _, addr := range addrs {
		if ip, ok := addr.(*net.IPNet); ok && !ip.IP.IsLoopback() {
			if ipv4 := ip.IP.To4(); ipv4 != nil {
				return ipv4.String()
			}
		}
	}
	return "127.0.0.1"
}

func ipToID(ip string) int {
	parts := strings.Split(ip, ".")
	if len(parts) == 4 {
		if id, err := strconv.Atoi(parts[3]); err == nil {
			return id
		}
	}
	return 0
}

func main() {
	id := flag.Int("id", 0, "unique node ID (e.g. 1); if 0, auto-assigned from IP")
	port := flag.Int("port", 0, "port to listen on (e.g. 5001); if 0, auto-assigned as 5000+ID")
	bind := flag.String("bind", "0.0.0.0", "bind address for HTTP server (use 0.0.0.0 for all interfaces)")
	advertise := flag.String("advertise", "", "advertised host/ip for this node (what peers should dial); if empty, auto-detected")
	peers := flag.String("peers", "", "comma-separated peer addresses host:port")
	autoJoin := flag.Bool("auto-join", false, "if true, automatically join cluster via first reachable peer on startup")
	flag.Parse()

	localIP := getLocalIP()
	if *id == 0 {
		*id = ipToID(localIP)
		if *id == 0 {
			fmt.Fprintln(os.Stderr, "Failed to auto-assign ID from IP, please provide --id")
			os.Exit(2)
		}
	}
	if *port == 0 {
		*port = 5000 + *id
	}

	if *advertise == "" {
		*advertise = fmt.Sprintf("%s:%d", localIP, *port)
	}

	peerList := []string{}
	if strings.TrimSpace(*peers) != "" {
		for _, p := range strings.Split(*peers, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				peerList = append(peerList, p)
			}
		}
	}

	n := internal.NewNode(internal.NodeConfig{
		ID:       *id,
		Port:     *port,
		Bind:     *bind,
		Host:     *advertise,
		Peers:    peerList,
		AutoJoin: *autoJoin,
	})

	n.Start()
}
