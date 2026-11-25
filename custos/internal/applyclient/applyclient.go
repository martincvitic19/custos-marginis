// Package applyclient is custosd's side of the privilege split: custosd
// (no root) writes custos.json and asks custos-apply (root) to apply it over
// a unix socket. The request carries no data; custos-apply re-reads and
// validates custos.json itself.
package applyclient

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"time"
)

// Socket is opened by systemd (custos-apply.socket), group custosd.
const Socket = "/run/custos/apply.sock"

// Status is custos-apply's answer (a subset of apply.Status).
type Status struct {
	OK      bool     `json:"ok"`
	Error   string   `json:"error,omitempty"`
	Changes []string `json:"changes"`
}

// Apply asks for a full apply and waits for the result.
func Apply(socket string) (*Status, error) {
	c, err := net.DialTimeout("unix", socket, 3*time.Second)
	if err != nil {
		return nil, fmt.Errorf("custos-apply socket: %w", err)
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(2 * time.Minute))
	if _, err := c.Write([]byte(`{"action":"apply"}` + "\n")); err != nil {
		return nil, err
	}
	line, err := bufio.NewReader(c).ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("custos-apply: no answer: %w", err)
	}
	var st Status
	if err := json.Unmarshal(line, &st); err != nil {
		return nil, err
	}
	return &st, nil
}
