from pydantic import BaseModel, Field


class WorkloadConfig(BaseModel):
    duration_secs: int = 30
    concurrency: int = 8
    db_url: str = ""
    hot_rows: int = 0


class WorkloadRequest(BaseModel):
    type: str
    mode: str = ""
    config: WorkloadConfig = Field(default_factory=WorkloadConfig)


class WorkloadResponse(BaseModel):
    run_id: str
    status: str


class StopResponse(BaseModel):
    status: str


class HealthResponse(BaseModel):
    status: str
    language: str
    mode: str


class AgentInfo(BaseModel):
    language: str
    library_version: str
    driver: str


class MetricSnapshot(BaseModel):
    ts: int = 0
    run_id: str = ""
    language: str = "python"
    mode: str = ""
    workload: str = ""
    ops_total: int = 0
    ops_per_sec: float = 0.0
    latency_p50_us: float = 0.0
    latency_p95_us: float = 0.0
    latency_p99_us: float = 0.0
    latency_max_us: float = 0.0
    errors: int = 0
    retries: int = 0
    retry_success: int = 0
    circuit_breaker_open: bool = False
    active_connections: int = 0
    cpu_percent: float = 0.0
    mem_rss_mb: float = 0.0
