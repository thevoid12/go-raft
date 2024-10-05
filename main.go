// Entry point of the application.
// Initializes nodes and starts their servers.

package main

import (
	"flag"
	"fmt"
	"go-raft/config"
	"go-raft/node"
)

// Retrieve configuration settings (like node addresses).
// Initializes and starts each node in a separate goroutine so that each can run independently and sim
// Waits indefinitely to keep the main process alive.
func main() {
	id := flag.Int("id", 0, "Node ID")
	address := flag.String("addr", "localhost:8001", "Node address")
	flag.Parse()

	cfg := config.GetConfig()

	n := node.NewNode(*id, cfg.Peers, *address, cfg)
	n.Start()

	fmt.Printf("Raft node %d is running at %s\n", *id, *address)
	select {} // Block forever
}
