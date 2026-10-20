package protocol

import (
	"encoding/json"
	"time"
)

type WorkloadType string

const (
	WorkloadCRUD          WorkloadType = "crud"
	WorkloadRetryStorm    WorkloadType = "retry_storm"
	WorkloadMixedRW       WorkloadType = "mixed_rw"
	WorkloadFollowerReads WorkloadType = "follower_reads"
	WorkloadCDC           WorkloadType = "cdc"
	WorkloadVectorSearch  WorkloadType = "vector_search"
)

type RunMode string

const (
	ModeLibrary RunMode = "library"
	ModeVanilla RunMode = "vanilla"
)

type Language string

const (
	LangJava   Language = "java"
	LangGo     Language = "go"
	LangPython Language = "python"
	LangDotNet     Language = "dotnet"
	LangJavaNative Language = "java-native"
)

type Topology string

const (
	TopoSingle  Topology = "single"
	TopoCluster Topology = "cluster"
	TopoMulti   Topology = "multi"
)

type ChaosScenario string

const (
	ChaosNodeKill         ChaosScenario = "node_kill"
	ChaosNetworkPartition ChaosScenario = "network_partition"
	ChaosRegionFailure    ChaosScenario = "region_failure"
	ChaosLatencyInjection ChaosScenario = "latency_injection"
	ChaosRollingRestart   ChaosScenario = "rolling_restart"
	ChaosDiskPressure     ChaosScenario = "disk_pressure"
)

type ChaosMode string

const (
	ChaosModeIndividual ChaosMode = "individual"
	ChaosModeGauntlet   ChaosMode = "gauntlet"
	ChaosModeRandom     ChaosMode = "random"
)

type WorkloadRequest struct {
	Type   WorkloadType    `json:"type"`
	Mode   RunMode         `json:"mode"`
	Config WorkloadConfig  `json:"config"`
}

type WorkloadConfig struct {
	DurationSecs int             `json:"duration_secs"`
	Concurrency  int             `json:"concurrency"`
	DBURL        string          `json:"db_url"`
	HotRows      int             `json:"hot_rows,omitempty"`
	WarmupSecs   int             `json:"warmup_secs,omitempty"`
	ScenarioData json.RawMessage `json:"scenario_data,omitempty"`
}

type WorkloadResponse struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

type StopResponse struct {
	Status string `json:"status"`
}

type HealthResponse struct {
	Status   string `json:"status"`
	Language string `json:"language"`
	Mode     string `json:"mode"`
}

type AgentInfo struct {
	Language       string `json:"language"`
	LibraryVersion string `json:"library_version"`
	Driver         string `json:"driver"`
}

type ErrorSample struct {
	SQLState string `json:"sqlstate"`
	Message  string `json:"message"`
	Count    int64  `json:"count"`
}

type MetricSnapshot struct {
	Timestamp        int64   `json:"ts"`
	RunID            string  `json:"run_id"`
	Language         string  `json:"language"`
	Mode             string  `json:"mode"`
	Workload         string  `json:"workload"`
	OpsTotal         int64   `json:"ops_total"`
	OpsPerSec        float64 `json:"ops_per_sec"`
	LatencyP50Us     float64 `json:"latency_p50_us"`
	LatencyP95Us     float64 `json:"latency_p95_us"`
	LatencyP99Us     float64 `json:"latency_p99_us"`
	LatencyMaxUs     float64 `json:"latency_max_us"`
	Errors           int64   `json:"errors"`
	Retries          int64   `json:"retries"`
	RetrySuccess     int64   `json:"retry_success"`
	CircuitBreakerOpen bool  `json:"circuit_breaker_open"`
	ActiveConnections  int   `json:"active_connections"`
	CPUPercent       float64 `json:"cpu_percent"`
	MemRSSMB         float64 `json:"mem_rss_mb"`
	ErrorSamples     []ErrorSample `json:"error_samples,omitempty"`
	Warmup           bool    `json:"warmup,omitempty"`
}

type ChaosEvent struct {
	Timestamp time.Time     `json:"timestamp"`
	Scenario  ChaosScenario `json:"scenario"`
	Action    string        `json:"action"`
	Target    string        `json:"target"`
	Duration  time.Duration `json:"duration_ms"`
}

type BenchmarkRun struct {
	ID           string          `json:"id"`
	Timestamp    time.Time       `json:"timestamp"`
	Topology     Topology        `json:"topology"`
	Workloads    []WorkloadType  `json:"workloads"`
	Duration     int             `json:"duration_secs"`
	Concurrency  int             `json:"concurrency,omitempty"`
	WarmupSecs   int             `json:"warmup_secs,omitempty"`
	Iterations   int             `json:"iterations,omitempty"`
	Languages    []string        `json:"languages,omitempty"`
	Isolation    bool            `json:"isolation,omitempty"`
	Agents       []AgentEntry    `json:"agents"`
	Chaos        []ChaosEvent    `json:"chaos_events,omitempty"`
	ScenarioJSON json.RawMessage `json:"scenario_json,omitempty"`
}

type AgentEntry struct {
	Language Language `json:"language"`
	Mode     RunMode  `json:"mode"`
	Port     int      `json:"port"`
	Host     string   `json:"host"`
}

var DefaultAgents = []AgentEntry{
	{Language: LangJava, Mode: ModeLibrary, Port: 7001, Host: "localhost"},
	{Language: LangJava, Mode: ModeVanilla, Port: 7002, Host: "localhost"},
	{Language: LangJavaNative, Mode: ModeLibrary, Port: 7009, Host: "localhost"},
	{Language: LangJavaNative, Mode: ModeVanilla, Port: 7010, Host: "localhost"},
	{Language: LangGo, Mode: ModeLibrary, Port: 7003, Host: "localhost"},
	{Language: LangGo, Mode: ModeVanilla, Port: 7004, Host: "localhost"},
	{Language: LangPython, Mode: ModeLibrary, Port: 7005, Host: "localhost"},
	{Language: LangPython, Mode: ModeVanilla, Port: 7006, Host: "localhost"},
	{Language: LangDotNet, Mode: ModeLibrary, Port: 7007, Host: "localhost"},
	{Language: LangDotNet, Mode: ModeVanilla, Port: 7008, Host: "localhost"},
}
