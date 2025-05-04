package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"gopkg.in/yaml.v2"
)

type Config struct {
	ChainBaseNodes    []string `yaml:"chain_base_nodes"`
	FilePathNodeId    string   `yaml:"filepath_node_id"`
	PubKey            string   `yaml:"pub_key"`
	HeartbeatInterval int      `yaml:"heartbeat_interval"`
	Influx            struct {
		URL    string `yaml:"url"`
		Bucket string `yaml:"bucket"`
		Org    string `yaml:"org"`
		Token  string `yaml:"token"`
	} `yaml:"influx"`
}

type Metrics struct {
	CPUPercent   float64 `json:"cpu_percent"`
	MemTotal     uint64  `json:"mem_total"`
	MemAvailable uint64  `json:"mem_available"`
	DiskTotal    uint64  `json:"disk_total"`
	DiskUsed     uint64  `json:"disk_used"`
	NetBytesSent uint64  `json:"net_bytes_sent"`
	NetBytesRecv uint64  `json:"net_bytes_recv"`
	Timestamp    int64   `json:"timestamp"`
}

type CommandRequest struct {
	NodeID  string   `json:"targetNode"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
	OwnerID string   `json:"ownerId"`
}

type CommandResult struct {
	Type      string `json:"type"`
	NodeID    string `json:"nodeID"`
	Command   string `json:"command"`
	Success   bool   `json:"success"`
	Output    string `json:"output"`
	Timestamp int64  `json:"timestamp"`
	OwnerID   string `json:"ownerId"`
}

var (
	cfg    *Config
	nodeID string
)

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func getOrCreateNodeID() string {
	if data, err := os.ReadFile(cfg.FilePathNodeId); err == nil {
		return string(data)
	}
	id := uuid.NewString()
	if err := os.WriteFile(cfg.FilePathNodeId, []byte(id), 0600); err != nil {
		log.Printf("Warning: cannot save nodeID: %v", err)
	}
	fmt.Printf("✅ Agent installed. Your nodeID is: %s\n", id)
	return id
}

func postToAnyNode(path string, body []byte) error {
	for _, base := range cfg.ChainBaseNodes {
		resp, err := http.Post(base+path, "application/json", bytes.NewReader(body))
		if err == nil && resp.StatusCode == 200 {
			resp.Body.Close()
			return nil
		}
	}
	return fmt.Errorf("failed to post to any node")
}

func collectMetrics() (*Metrics, error) {
	ts := time.Now().Unix()
	cpuPct, err := cpu.Percent(0, false)
	if err != nil {
		return nil, err
	}
	vm, err := mem.VirtualMemory()
	if err != nil {
		return nil, err
	}
	du, err := disk.Usage("/")
	if err != nil {
		return nil, err
	}
	ioStats, err := net.IOCounters(false)
	if err != nil {
		return nil, err
	}

	return &Metrics{
		CPUPercent:   cpuPct[0],
		MemTotal:     vm.Total,
		MemAvailable: vm.Available,
		DiskTotal:    du.Total,
		DiskUsed:     du.Used,
		NetBytesSent: ioStats[0].BytesSent,
		NetBytesRecv: ioStats[0].BytesRecv,
		Timestamp:    ts,
	}, nil
}

func pushToInflux(m *Metrics) {
	tsNano := time.Now().UnixNano()
	line := fmt.Sprintf(
		"server_metrics,nodeID=%s cpu=%f,mem_total=%d,mem_available=%d,disk_total=%d,disk_used=%d,net_sent=%d,net_recv=%d %d",
		nodeID,
		m.CPUPercent,
		m.MemTotal,
		m.MemAvailable,
		m.DiskTotal,
		m.DiskUsed,
		m.NetBytesSent,
		m.NetBytesRecv,
		tsNano,
	)
	url := fmt.Sprintf(
		"%s/api/v2/write?org=%s&bucket=%s&precision=ns",
		cfg.Influx.URL, cfg.Influx.Org, cfg.Influx.Bucket,
	)
	req, _ := http.NewRequest("POST", url, bytes.NewBufferString(line))
	req.Header.Set("Authorization", "Token "+cfg.Influx.Token)
	req.Header.Set("Content-Type", "text/plain")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Influx write error: %v", err)
		return
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
}

func sendHeartbeat() {
	m, err := collectMetrics()
	if err != nil {
		log.Printf("Metrics error: %v", err)
	} else {
		pushToInflux(m)
	}
	data, _ := json.Marshal(m)
	hash := sha256.Sum256(data)
	tx := map[string]interface{}{
		"type":        "Heartbeat",
		"nodeID":      nodeID,
		"metricsHash": hex.EncodeToString(hash[:]),
		"timestamp":   time.Now().Unix(),
	}
	body, _ := json.Marshal(tx)
	if err := postToAnyNode("/broadcast_tx", body); err != nil {
		log.Printf("Heartbeat error: %v", err)
	}
}

func fetchCommands() []CommandRequest {
	for _, base := range cfg.ChainBaseNodes {
		url := fmt.Sprintf("%s/commands?nodeID=%s", base, nodeID)
		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		defer resp.Body.Close()
		var cmds []CommandRequest
		if err := json.NewDecoder(resp.Body).Decode(&cmds); err == nil {
			return cmds
		}
	}
	return nil
}

func handleCommands(cmds []CommandRequest) {
	for _, c := range cmds {
		out, err := exec.Command(c.Command, c.Args...).CombinedOutput()
		result := CommandResult{
			Type:      "CommandResult",
			NodeID:    nodeID,
			Command:   c.Command,
			Success:   err == nil,
			Output:    string(out),
			Timestamp: time.Now().Unix(),
			OwnerID:   c.OwnerID,
		}
		b, _ := json.Marshal(result)
		if err := postToAnyNode("/broadcast_tx", b); err != nil {
			log.Printf("Result send err: %v", err)
		}
	}
}

func registerNode() {
	tx := map[string]interface{}{
		"type":   "RegisterNode",
		"nodeID": nodeID,
		"pubKey": cfg.PubKey,
	}
	b, _ := json.Marshal(tx)
	if err := postToAnyNode("/broadcast_tx", b); err != nil {
		log.Printf("RegisterNode error: %v", err)
	} else {
		log.Printf("✅ Registered nodeID: %s", nodeID)
	}
}

func main() {
	var err error
	cfg, err = loadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Config error: %v", err)
	}

	firstRun := !fileExists(cfg.FilePathNodeId)
	nodeID = getOrCreateNodeID()
	if firstRun {
		registerNode()
	}

	ticker := time.NewTicker(time.Duration(cfg.HeartbeatInterval) * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		sendHeartbeat()
		handleCommands(fetchCommands())
	}
}
