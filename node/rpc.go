package node

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go-raft/config"
	"go-raft/log"
	"io"
	golog "log"
	"net/http"
	"strings"
)

// RequestVote RPC structures
type RequestVoteRequest struct {
	Term         int `json:"term"`
	CandidateID  int `json:"candidate_id"`
	LastLogIndex int `json:"last_log_index"`
	LastLogTerm  int `json:"last_log_term"`
}

type RequestVoteResponse struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"vote_granted"`
}

// AppendEntries RPC structures (used for heartbeats)
type AppendEntriesRequest struct {
	Term         int            `json:"term"`
	LeaderID     int            `json:"leader_id"`
	PrevLogIndex int            `json:"prev_log_index"`
	PrevLogTerm  int            `json:"prev_log_term"`
	Entries      []log.LogEntry `json:"entries"`
	LeaderCommit int            `json:"leader_commit"`
}

type AppendEntriesResponse struct {
	Term       int  `json:"term"`
	Success    bool `json:"success"`
	MatchIndex int  `json:"match_index,omitempty"`
}

// HandleRequestVote processes incoming RequestVote RPCs
func (n *Node) HandleRequestVote(w http.ResponseWriter, r *http.Request) {
	var req RequestVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid RequestVote Request", http.StatusBadRequest)
		return
	}

	// Validate CandidateID and Term
	if req.CandidateID <= 0 || req.Term <= 0 {
		http.Error(w, "Invalid CandidateID or Term", http.StatusBadRequest)
		golog.Printf("Node %d: Received invalid RequestVote from Candidate %d for term %d", n.id, req.CandidateID, req.Term)
		return
	}

	golog.Printf("Node %d: Received RequestVote from Candidate %d for term %d", n.id, req.CandidateID, req.Term)

	n.mu.Lock()
	defer n.mu.Unlock()

	resp := RequestVoteResponse{
		Term:        n.currentTerm,
		VoteGranted: false,
	}

	if req.Term < n.currentTerm {
		// Reply false if term is outdated
	} else {
		if req.Term > n.currentTerm {
			// Update term if request has a higher term
			n.currentTerm = req.Term
			n.votedFor = -1
			n.state = Follower
			golog.Printf("Node %d: Term updated to %d by RequestVote from %d", n.id, n.currentTerm, req.CandidateID)
		}

		// Check if we can grant a vote based on log consistency
		if (n.votedFor == -1 || n.votedFor == req.CandidateID) &&
			(req.LastLogTerm > n.log.GetLastTerm() ||
				(req.LastLogTerm == n.log.GetLastTerm() && req.LastLogIndex >= n.log.GetLastIndex())) {
			// Grant vote
			n.votedFor = req.CandidateID
			resp.VoteGranted = true
			n.resetElectionTimeout()
			golog.Printf("Node %d: Granted vote to Candidate %d for term %d", n.id, req.CandidateID, req.Term)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// SendRequestVote sends a RequestVote RPC to a peer
func (n *Node) SendRequestVote(peer string, req RequestVoteRequest) (RequestVoteResponse, error) {
	var resp RequestVoteResponse

	body, err := json.Marshal(req)
	if err != nil {
		return resp, err
	}
	cfg := config.GetConfig()
	client := &http.Client{
		Timeout: 2 * cfg.Timeout, // Increase timeout to avoid early failures
	}

	httpResp, err := client.Post("http://"+peer+"/request_vote", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("received non-OK status code: %d", httpResp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return resp, err
	}

	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		return resp, err
	}

	return resp, nil
}

// SendAppendEntries sends a heartbeat (AppendEntries RPC) to a peer
func (n *Node) SendAppendEntries(peer string, req AppendEntriesRequest) (AppendEntriesResponse, error) {
	var resp AppendEntriesResponse

	body, err := json.Marshal(req)
	if err != nil {
		return resp, err
	}

	cfg := config.GetConfig()
	client := &http.Client{
		Timeout: 2 * cfg.Timeout, // Increase timeout to avoid network delays
	}

	httpResp, err := client.Post("http://"+peer+"/append_entries", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return resp, fmt.Errorf("received non-OK status code: %d", httpResp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return resp, err
	}

	if err := json.Unmarshal(bodyBytes, &resp); err != nil {
		return resp, err
	}

	return resp, nil
}

// HandleAppendEntries handles incoming AppendEntries RPCs (heartbeats and log replication)
func (n *Node) HandleAppendEntries(w http.ResponseWriter, r *http.Request) {
	var req AppendEntriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid AppendEntries request", http.StatusBadRequest)
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	resp := AppendEntriesResponse{
		Term:    n.currentTerm,
		Success: false,
	}

	if req.Term < n.currentTerm {
		// Reply false if term < currentTerm
		json.NewEncoder(w).Encode(resp)
		return
	}

	// If RPC request contains term >= currentTerm, update currentTerm and state
	if req.Term > n.currentTerm {
		n.currentTerm = req.Term
		n.state = Follower
		n.votedFor = -1 // Reset votedFor when a higher term is encountered
	} else if req.Term == n.currentTerm {
		// If term is the same, ensure the node is a follower
		n.state = Follower
	}

	// Set leaderID to the sender of the heartbeat
	n.leaderID = req.LeaderID

	// Reset election timeout since a valid leader is communicating
	n.resetElectionTimeout()

	// Send signal to heartbeatCh to indicate a heartbeat was received
	select {
	case n.heartbeatCh <- struct{}{}:
		// Heartbeat sent to heartbeatch
		golog.Printf("Heartbeat sent to heartbeatch")
	default:
		golog.Printf("Node %d: Heartbeat channel already full, skipping send", n.id)
	}

	// Simplified log consistency check
	if req.PrevLogIndex > 0 {
		if req.PrevLogIndex > n.log.GetLastIndex() {
			json.NewEncoder(w).Encode(resp)
			return
		}
		prevEntry, ok := n.log.GetEntry(req.PrevLogIndex - 1) // Zero-based
		if !ok || prevEntry.Term != req.PrevLogTerm {
			// Mismatch in logs, truncate and reject the AppendEntries request
			n.log.Truncate(req.PrevLogIndex)
			json.NewEncoder(w).Encode(resp)
			return
		}
	}

	// Append any new entries not already in the log
	for i, entry := range req.Entries {
		index := req.PrevLogIndex + i
		if index < n.log.GetLastIndex() {
			existingEntry, _ := n.log.GetEntry(index - 1)
			if existingEntry.Term != entry.Term {
				// Delete the existing entry and all that follow it
				n.log.Truncate(index)
				n.log.Append(entry)
			}
		} else {
			n.log.Append(entry)
		}
	}

	// Update commitIndex
	if req.LeaderCommit > n.commitIndex {
		lastNewEntry := req.PrevLogIndex + len(req.Entries)
		if req.LeaderCommit < lastNewEntry {
			n.commitIndex = req.LeaderCommit
		} else {
			n.commitIndex = lastNewEntry
		}
		// Apply entries up to commitIndex
		for n.lastApplied < n.commitIndex {
			n.lastApplied++
			entry, _ := n.log.GetEntry(n.lastApplied - 1)
			n.apply(entry)
			golog.Printf("Node %d applied command '%s' at index %d", n.id, entry.Command, n.lastApplied)
		}
	}

	resp.Success = true
	json.NewEncoder(w).Encode(resp)
}

// apply applies a committed log entry to the state machine
func (n *Node) apply(entry log.LogEntry) {
	// Simple key-value command parsing
	parts := strings.Split(entry.Command, "=")
	if len(parts) != 2 {
		golog.Printf("Node %d: Failed to parse command '%s': invalid format", n.id, entry.Command)
		return
	}

	key := strings.TrimSpace(parts[0][4:]) // Remove "set " from the key
	value := strings.TrimSpace(parts[1])   // Trim spaces from the value

	n.stateMachine[key] = value
	golog.Printf("Node %d: Applied command: set %s=%s", n.id, key, value)
}
