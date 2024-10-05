// Handles Remote Procedure Calls (RPCs) like RequestVote and AppendEntries and basically
// handles all the heartbeat logics here

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
)

// RequestVote RPC structures
type RequestVoteRequest struct {
	Term        int `json:"term"`
	CandidateID int `json:"candidate_id"`
	// TODO: Additional fields like LastLogIndex and LastLogTerm can be added
}

type RequestVoteResponse struct {
	Term        int  `json:"term"`
	VoteGranted bool `json:"vote_granted"`
}

// AppendEntries RPC structures (used for heartbeats)
type AppendEntriesRequest struct {
	Term         int            `json:"term"`
	LeaderID     int            `json:"leader_id"`
	PrevLogIndex int            `json:"prev_log_index"` //Ensure log consistency by checking the previous log entry.
	PrevLogTerm  int            `json:"prev_log_term"`  //Ensure log consistency by checking the previous log entry.
	Entries      []log.LogEntry `json:"entries"`
	LeaderCommit int            `json:"leader_commit"`
}

type AppendEntriesResponse struct {
	Term    int  `json:"term"`
	Success bool `json:"success"`
	// For optimization, include the index up to which entries are matched
	MatchIndex int `json:"match_index,omitempty"`
}

// HandleRequestVote processes incoming RequestVote RPCs
func (n *Node) HandleRequestVote(w http.ResponseWriter, r *http.Request) {
	var req RequestVoteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid RequestVote Request", http.StatusBadRequest)
		return
	}

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
			n.currentTerm = req.Term
			n.votedFor = -1
			n.state = Follower
		}

		if n.votedFor == -1 || n.votedFor == req.CandidateID {
			// Grant vote
			n.votedFor = req.CandidateID
			resp.VoteGranted = true
			// Reset election timeout
			n.resetElectionTimeout()
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
		Timeout: cfg.Timeout,
	}

	httpResp, err := client.Post("http://"+peer+"/request_vote", "application/json", bytes.NewBuffer(body))
	if err != nil {
		return resp, err
	}
	defer httpResp.Body.Close()

	if httpResp.StatusCode != http.StatusOK {
		return resp, err
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
	client := &http.Client{ //it will try until the timeout to fullfil the request
		Timeout: cfg.Timeout,
	}

	httpResp, err := client.Post("http://"+peer+"/append_entries", "application/json", bytes.NewBuffer(body)) // Correct endpoint
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

	golog.Printf("Node %d received AppendEntries from Leader %d for term %d", n.id, req.LeaderID, req.Term)

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
		n.votedFor = -1
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
		golog.Printf("Node %d: Heartbeat signal sent to heartbeatCh", n.id)
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
	// Example command: "set x=1"
	var key, value string
	_, err := fmt.Sscanf(entry.Command, "set %s=%s", &key, &value)
	if err != nil {
		golog.Printf("Node %d: Failed to parse command '%s': %v", n.id, entry.Command, err)
		return
	}

	n.stateMachine[key] = value
	golog.Printf("Node %d: Applied command: set %s=%s", n.id, key, value)
}
