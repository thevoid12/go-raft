// Defines the Node struct and its core methods (e.g., state transitions).
package node

import (
	"go-raft/config"
	"go-raft/log"
	"go-raft/utils"
	golog "log"
	"net/http"
	"sync"
	"time"
)

type Node struct {
	mu                 sync.Mutex
	id                 int
	state              State
	currentTerm        int
	votedFor           int
	peers              []string
	address            string
	leaderID           int
	electionTimeoutMin time.Duration
	electionTimeoutMax time.Duration
	electionTimeout    time.Duration
	heartbeatInterval  time.Duration

	log         *log.Log
	commitIndex int
	lastApplied int

	server *http.Server

	// Channels for signaling
	heartbeatCh chan bool
	shutdownCh  chan struct{}
}

// Initializes node state, including term, votedFor, and log.
// Initially everybody is a follower and there wont be any leader because
//
//	when initiating there wont be any leader selection until the first election timeout
func NewNode(id int, peers []string, address string, cfg config.Config) *Node {
	return &Node{
		id:                 id,
		state:              Follower,
		currentTerm:        0,
		votedFor:           -1,
		peers:              peers,
		address:            address,
		leaderID:           -1,
		heartbeatInterval:  cfg.HeartbeatInterval,
		electionTimeoutMin: cfg.ElectionTimeoutMin,
		electionTimeoutMax: cfg.ElectionTimeoutMax,
		electionTimeout:    utils.RandomElectionTimeout(cfg.ElectionTimeoutMin, cfg.ElectionTimeoutMax),
		log:                log.NewLog(),
		commitIndex:        0,
		lastApplied:        0,
		heartbeatCh:        make(chan bool),     // Signals receipt of a heartbeat.
		shutdownCh:         make(chan struct{}), // Signals when the node should shut down.
	}
}

// Sets up HTTP routes for RPCs and client interactions and
//
// starts the HTTP server and begins the main event loop.
func (n *Node) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/heartbeat", n.HandleHeartbeat)
	mux.HandleFunc("/request_vote", n.HandleRequestVote)
	mux.HandleFunc("/client", n.HandleClientRequest)

	n.server = &http.Server{
		Addr:    n.address,
		Handler: mux,
	}

	go func() {
		if err := n.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			golog.Fatalf("Node %d: ListenAndServe() error: %v", n.id, err)
		}
	}()

	golog.Printf("Node %d started at %s", n.id, n.address)
	n.run()
}
