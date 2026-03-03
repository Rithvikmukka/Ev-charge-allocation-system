package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"distributed-ev/internal"
)

func main() {
	id := flag.Int("id", 0, "unique node ID (e.g. 1)")
	port := flag.Int("port", 0, "port to listen on (e.g. 5001)")
	bind := flag.String("bind", "0.0.0.0", "bind address for HTTP server (use 0.0.0.0 for all interfaces)")
	advertise := flag.String("advertise", "localhost", "advertised host/ip for this node (what peers should dial)")
	peers := flag.String("peers", "", "comma-separated peer addresses host:port")
	flag.Parse()

	if *id <= 0 || *port <= 0 {
		fmt.Fprintln(os.Stderr, "Usage: go run ./cmd --id=1 --port=5001 [--bind=0.0.0.0] [--advertise=192.168.1.10] --peers=192.168.1.11:5002,192.168.1.12:5003")
		os.Exit(2)
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
		ID:    *id,
		Port:  *port,
		Bind:  *bind,
		Host:  *advertise,
		Peers: peerList,
	})

	n.Start()
}
