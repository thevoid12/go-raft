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
			golog.Printf("Node %d: State changed to Follower", n.id)
			n.runFollower()
		case Candidate:
			golog.Printf("Node %d: State changed to candidate", n.id)
			n.runCandidate()
		case Leader:
			n.runLeader()
		}
	}
}

func (n *Node) runFollower() {
	timer := time.NewTimer(n.electionTimeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			n.mu.Lock()
			n.state = Candidate
			n.mu.Unlock()
			golog.Printf("Node %d promoting to candidate as no heartbeat is received/election timeout", n.id)
			return // Exit to start candidate logic

		case <-n.heartbeatCh:
			n.mu.Lock()
			// Reset election timeout if heartbeat received
			if !timer.Stop() {
				<-timer.C // Drain the timer if it fired
			}
			n.resetElectionTimeout()
			timer.Reset(n.electionTimeout)
			golog.Printf("Node %d received heartbeat from leader %d", n.id, n.leaderID)
			n.mu.Unlock()

		case <-n.shutdownCh:
			golog.Printf("Node %d shutting down", n.id)
			return
		}
	}
}

func (n *Node) runCandidate() {
	n.mu.Lock()
	// Increment the term when transitioning to candidate
	n.currentTerm += 1
	currentTerm := n.currentTerm
	n.votedFor = n.id
	n.resetElectionTimeout()
	golog.Printf("Node %d: Promoting to Candidate for term %d", n.id, currentTerm)
	n.mu.Unlock()

	votes := 1 // Vote for self
	var voteMu sync.Mutex
	var wg sync.WaitGroup

	for i, peer := range n.peers {
		peerID := i + 1 // Adjust for 1-based Node IDs
		if peerID == n.id {
			continue // Skip self
		}
		wg.Add(1)
		go func(peer string) {
			defer wg.Done()
			lastLogIndex := n.log.GetLastIndex()
			lastLogTerm := n.log.GetLastTerm()

			req := RequestVoteRequest{
				Term:         currentTerm,
				CandidateID:  n.id,
				LastLogIndex: lastLogIndex,
				LastLogTerm:  lastLogTerm,
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
				golog.Printf("Node %d: Term updated to %d by ResponseVote from %s", n.id, n.currentTerm, peer)
				return
			}

			if resp.VoteGranted {
				voteMu.Lock()
				votes += 1
				golog.Printf("Node %d: Received vote from %s (Total votes: %d)", n.id, peer, votes)
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
		golog.Printf("************************************************************")
		golog.Printf("Node %d became Leader for term %d with %d votes", n.id, n.currentTerm, votes)
		golog.Printf("************************************************************")

		// Initialize nextIndex and matchIndex
		lastLogIndex := n.log.GetLastIndex()
		n.nextIndex = make([]int, len(n.peers))
		n.matchIndex = make([]int, len(n.peers))
		for i := range n.peers {
			n.nextIndex[i] = lastLogIndex + 1
			n.matchIndex[i] = 0
		}

		go n.startHeartbeats()
	} else {
		n.state = Follower
		n.resetElectionTimeout()
		golog.Printf("Node %d: Did not receive majority (%d votes). Reverting to Follower", n.id, votes)
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
		select {
		case <-ticker.C:
			n.mu.Lock()
			if n.state != Leader {
				n.mu.Unlock()
				return
			}
			currentTerm := n.currentTerm
			n.mu.Unlock()

			var wg sync.WaitGroup
			for i, peer := range n.peers {
				peerID := i + 1 // Assuming node IDs start at 1
				if peerID == n.id {
					continue //skip heartbeat to self
				}
				wg.Add(1)
				go func(i int, peer string) {
					defer wg.Done()

					n.mu.Lock()
					prevLogIndex := n.nextIndex[i] - 1
					prevLogTerm := 0
					if prevLogIndex > 0 {
						entry, ok := n.log.GetEntry(prevLogIndex - 1) // Zero-based indexing
						if ok {
							prevLogTerm = entry.Term
						}
					}

					entries := []log.LogEntry{}
					if n.nextIndex[i]-1 < n.log.GetLastIndex() {
						allEntries := n.log.GetEntries()
						if n.nextIndex[i]-1 < len(allEntries) {
							entries = allEntries[n.nextIndex[i]-1:]
						}
					}

					req := AppendEntriesRequest{
						Term:         currentTerm,
						LeaderID:     n.id,
						PrevLogIndex: prevLogIndex,
						PrevLogTerm:  prevLogTerm,
						Entries:      entries,
						LeaderCommit: n.commitIndex,
					}
					n.mu.Unlock()

					golog.Printf("Leader %d sending heartbeat to %s", n.id, peer)
					resp, err := n.SendAppendEntries(peer, req)
					if err != nil {
						golog.Printf("Leader %d: Failed to send AppendEntries to %s: %v", n.id, peer, err)
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

					if resp.Success {
						n.nextIndex[i] = prevLogIndex + len(entries) + 1
						n.matchIndex[i] = n.nextIndex[i] - 1

						// Update commitIndex
						for idx := n.commitIndex + 1; idx <= n.log.GetLastIndex(); idx++ {
							entry, ok := n.log.GetEntry(idx - 1)
							if !ok || entry.Term != n.currentTerm {
								continue
							}
							count := 1 // Count the leader itself
							for j := range n.peers {
								peerID := j + 1 // Assuming node IDs start at 1
								if peerID == n.id {
									continue
								}

								if n.matchIndex[j] >= idx {
									count++
								}
							}
							if count > len(n.peers)/2 {
								n.commitIndex = idx
								n.apply(entry)
								golog.Printf("Leader %d: Committed index %d", n.id, n.commitIndex)
							}
						}
					} else {
						if n.nextIndex[i] > 1 {
							n.nextIndex[i]--
						}
					}

				}(i, peer)
			}

			wg.Wait()
		case <-n.shutdownCh:
			return
		}
	}
}

// resetElectionTimeout resets the election timeout with a new random duration
func (n *Node) resetElectionTimeout() {
	n.electionTimeout = utils.RandomElectionTimeout(n.electionTimeoutMin, n.electionTimeoutMax)
}

func (n *Node) HandleInspect(w http.ResponseWriter, r *http.Request) {
	n.mu.Lock()
	defer n.mu.Unlock()

	response := struct {
		State        string            `json:"state"`
		CurrentTerm  int               `json:"current_term"`
		VotedFor     int               `json:"voted_for"`
		CommitIndex  int               `json:"commit_index"`
		LastApplied  int               `json:"last_applied"`
		LogEntries   []log.LogEntry    `json:"log_entries"`
		StateMachine map[string]string `json:"state_machine"`
	}{
		State:        n.state.String(),
		CurrentTerm:  n.currentTerm,
		VotedFor:     n.votedFor,
		CommitIndex:  n.commitIndex,
		LastApplied:  n.lastApplied,
		LogEntries:   n.log.GetEntries(),
		StateMachine: n.stateMachine,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
