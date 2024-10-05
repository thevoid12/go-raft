// Handles Remote Procedure Calls (RPCs) like RequestVote and AppendEntries and basically
// handles all the heartbeat logics here

package node

import (
	"bytes"
	"encoding/json"
	"io"

	"net/http"
	"time"
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
	Term     int `json:"term"`
	LeaderID int `json:"leader_id"`
	// TODO: Additional fields like PrevLogIndex, PrevLogTerm, Entries, LeaderCommit can be added
}

type AppendEntriesResponse struct {
	Term    int  `json:"term"`
	Success bool `json:"success"`
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

// HandleHeartbeat processes incoming AppendEntries RPCs (heartbeats)
func (n *Node) HandleHeartbeat(w http.ResponseWriter, r *http.Request) {
	var req AppendEntriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid AppendEntries Request", http.StatusBadRequest)
		return
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	resp := AppendEntriesResponse{
		Term:    n.currentTerm,
		Success: false,
	}

	if req.Term < n.currentTerm {
		// Reject heartbeat
	} else {
		if req.Term > n.currentTerm {
			n.currentTerm = req.Term
			n.votedFor = -1
		}
		n.state = Follower
		n.leaderID = req.LeaderID
		resp.Success = true
		// Reset election timeout
		n.resetElectionTimeout()
		// Signal heartbeat receipt
		select {
		case n.heartbeatCh <- true:
		default:
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

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
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

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}

	httpResp, err := client.Post("http://"+peer+"/heartbeat", "application/json", bytes.NewBuffer(body))
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
