package nodeagent

import (
	"context"
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Traffic is one measurement window read from xray's stats API.
type Traffic struct {
	Users    map[string]*Counter
	Inbounds map[string]*Counter
}

type Counter struct {
	Uplink   uint64
	Downlink uint64
}

// statsQueryResponse mirrors the JSON `xray api statsquery` prints.
type statsQueryResponse struct {
	Stat []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"stat"`
}

// CollectTraffic reads and resets the counters, so every call returns the delta
// since the previous one. Reading with reset is what keeps the agent stateless.
func (x *Xray) CollectTraffic(ctx context.Context) (*Traffic, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, x.binary, "api", "statsquery",
		"--server="+x.apiAddr, "-reset")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	var parsed statsQueryResponse
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, err
	}

	t := &Traffic{Users: map[string]*Counter{}, Inbounds: map[string]*Counter{}}
	for _, s := range parsed.Stat {
		// Names look like "user>>>alice>>>traffic>>>uplink".
		parts := strings.Split(s.Name, ">>>")
		if len(parts) != 4 || parts[2] != "traffic" {
			continue
		}
		value, err := strconv.ParseUint(s.Value, 10, 64)
		if err != nil || value == 0 {
			continue
		}

		var target map[string]*Counter
		switch parts[0] {
		case "user":
			target = t.Users
		case "inbound":
			target = t.Inbounds
		default:
			continue
		}

		c := target[parts[1]]
		if c == nil {
			c = &Counter{}
			target[parts[1]] = c
		}
		if parts[3] == "uplink" {
			c.Uplink += value
		} else {
			c.Downlink += value
		}
	}
	return t, nil
}
