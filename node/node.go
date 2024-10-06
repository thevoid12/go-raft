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
	lastReceivedTime   time.Time // Track last received value from leader

	log         *log.Log
	commitIndex int
	lastApplied int

	server *http.Server

	// Channels for signaling
	heartbeatCh chan struct{}
	shutdownCh  chan struct{}
	// Leader-specific fields
	nextIndex    []int
	matchIndex   []int
	stateMachine map[string]string
}

// Initializes node state, including term, votedFor, and log.
// Initially everybody is a follower and there wont be any leader because
//
//	when initiating there wont be any leader selection until the first election timeout
func NewNode(id int, peers []string, address string, cfg config.Config) *Node {
	if id <= 0 || id > len(peers) {
		golog.Fatalf("Invalid node ID %d. Must be between 1 and %d.", id, len(peers))
	}
	return &Node{
		mu:                 sync.Mutex{},
		id:                 id,
		state:              Follower,
		currentTerm:        1,
		votedFor:           -1,
		peers:              peers,
		address:            address,
		leaderID:           -1,
		electionTimeoutMin: cfg.ElectionTimeoutMin,
		electionTimeoutMax: cfg.ElectionTimeoutMax,
		electionTimeout:    utils.RandomElectionTimeout(cfg.ElectionTimeoutMin, cfg.ElectionTimeoutMax),
		heartbeatInterval:  cfg.HeartbeatInterval,
		lastReceivedTime:   time.Time{},
		log:                log.NewLog(),
		commitIndex:        0,
		lastApplied:        0,
		server:             &http.Server{},
		heartbeatCh:        make(chan struct{}, 1),
		shutdownCh:         make(chan struct{}),
		nextIndex:          []int{},
		matchIndex:         []int{},
		stateMachine:       make(map[string]string),
	}
}

// Sets up HTTP routes for RPCs and client interactions and
//
// starts the HTTP server and begins the main event loop.
func (n *Node) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/append_entries", n.HandleAppendEntries)
	mux.HandleFunc("/request_vote", n.HandleRequestVote)
	mux.HandleFunc("/client", n.HandleClientRequest)
	mux.HandleFunc("/inspect", n.HandleInspect)

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
