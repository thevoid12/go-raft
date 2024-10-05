// Handles client interactions, such as processing POST requests to the leader.

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Client struct {
	leaderAddr string
}

func NewClient(leaderAddr string) *Client {
	return &Client{
		leaderAddr: leaderAddr,
	}
}

func (c *Client) SendCommand(command string) error {
	url := fmt.Sprintf("http://%s/client", c.leaderAddr)
	payload := map[string]string{
		"command": command,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to send command, status code: %d", resp.StatusCode)
	}

	return nil
}
