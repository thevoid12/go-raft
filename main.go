// Entry point of the application.
// Initializes nodes and starts their servers.

package main

import (
	"fmt"
	"raft/config"
	"raft/node"
	"sync"
)

// Retrieve configuration settings (like node addresses).
// Initializes and starts each node in a separate goroutine.
// Waits indefinitely to keep the main process alive.
func main() {
	cfg := config.GetConfig()

	var wg sync.WaitGroup
	wg.Add(len(cfg.Peers))

	for i, addr := range cfg.Peers {
		go func(id int, address string) {
			defer wg.Done()
			n := node.NewNode(id, cfg.Peers, address)
			n.Start()
		}(i, addr)
	}

	fmt.Println("Raft cluster is up and running......")
	wg.Wait()
}
