// Implementation of  the core Raft state machine logic, including state transitions and periodic tasks like elections and heartbeats.
package node

import (
	"encoding/json"
	"go-raft/log"
	"go-raft/utils"
	golog "log"
	"net/http"
	"sync"
	"time"
)

// HandleClientRequest handles client POST requests to the leader
func (n *Node) HandleClientRequest(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state != Leader {
		// Redirect to leader
		if n.leaderID != -1 {
			leaderAddr := n.peers[n.leaderID]
			http.Redirect(w, r, "http://"+leaderAddr+"/client", http.StatusTemporaryRedirect)
		} else {
			http.Error(w, "No leader elected", http.StatusServiceUnavailable)
		}
		return
	}

	// Read command from client
	var cmd struct {
		Command string `json:"command"`
	}
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		http.Error(w, "Invalid command", http.StatusBadRequest)
		return
	}

	// Append to leader's log
	entry := log.LogEntry{
		Term:    n.currentTerm,
		Command: cmd.Command,
	}
	n.log.Append(entry)

	// TODO: Implement log replication to followers
	// For simplicity, we'll assume immediate commit if leader

	n.commitIndex = len(n.log.GetEntries()) - 1
	golog.Printf("Leader %d committed command: %s", n.id, cmd.Command)

	w.WriteHeader(http.StatusOK)
}

// run contains the main loop for the node, handling state transitions
func (n *Node) run() {
	utils.Init()

	for {
		switch n.state {
		case Follower:
			n.runFollower()
		case Candidate:
			n.runCandidate()
		case Leader:
			n.runLeader()
		}
	}
}

func (n *Node) runFollower() {
	timer := time.NewTimer(n.electionTimeout)
	<-timer.C

	n.mu.Lock()
	n.state = Candidate
	n.mu.Unlock()
}

func (n *Node) runCandidate() {
	n.mu.Lock()
	n.currentTerm += 1
	n.votedFor = n.id
	currentTerm := n.currentTerm
	n.state = Candidate
	n.resetElectionTimeout()
	n.mu.Unlock()

	votes := 1
	var voteMu sync.Mutex
	var wg sync.WaitGroup

	for i, peer := range n.peers {
		if i == n.id {
			continue
		}
		wg.Add(1)
		go func(peer string) {
			defer wg.Done()
			req := RequestVoteRequest{
				Term:        currentTerm,
				CandidateID: n.id,
			}
			resp, err := n.SendRequestVote(peer, req)
			if err != nil {
				golog.Printf("Node %d: Failed to send RequestVote to %s: %v", n.id, peer, err)
				return
			}

			n.mu.Lock()
			defer n.mu.Unlock()

			if resp.Term > n.currentTerm {
				n.currentTerm = resp.Term
				n.state = Follower
				n.votedFor = -1
				return
			}

			if resp.VoteGranted {
				voteMu.Lock()
				votes += 1
				voteMu.Unlock()
			}
		}(peer)
	}

	wg.Wait()

	n.mu.Lock()
	defer n.mu.Unlock()

	if n.state != Candidate {
		return
	}

	if votes > len(n.peers)/2 {
		n.state = Leader
		n.leaderID = n.id
		golog.Printf("Node %d became Leader for term %d", n.id, n.currentTerm)
		go n.startHeartbeats()
	} else {
		n.state = Follower
		n.resetElectionTimeout()
	}
}

func (n *Node) runLeader() {
	// Leader operations are handled in startHeartbeats
	// No action needed here
}

func (n *Node) startHeartbeats() {
	ticker := time.NewTicker(n.heartbeatInterval)
	defer ticker.Stop()

	for {
		n.mu.Lock()
		if n.state != Leader {
			n.mu.Unlock()
			return
		}
		currentTerm := n.currentTerm
		n.mu.Unlock()

		var wg sync.WaitGroup
		for i, peer := range n.peers {
			if i == n.id {
				continue
			}
			wg.Add(1)
			go func(peer string) {
				defer wg.Done()
				req := AppendEntriesRequest{
					Term:     currentTerm,
					LeaderID: n.id,
					// Entries can be added for log replication
				}
				resp, err := n.SendAppendEntries(peer, req)
				if err != nil {
					golog.Printf("Leader %d: Failed to send heartbeat to %s: %v", n.id, peer, err)
					return
				}

				n.mu.Lock()
				defer n.mu.Unlock()

				if resp.Term > n.currentTerm {
					n.currentTerm = resp.Term
					n.state = Follower
					n.votedFor = -1
				}
			}(peer)
		}
		wg.Wait()

		time.Sleep(n.heartbeatInterval)
	}
}

// resetElectionTimeout resets the election timeout with a new random duration
func (n *Node) resetElectionTimeout() {
	n.electionTimeout = utils.RandomElectionTimeout()
}
