import bisect
import os
import threading
import time

import psutil

from agent.protocol import MetricSnapshot


class MetricsRecorder:
    def __init__(self) -> None:
        self._lock = threading.Lock()
        self._latencies: list[int] = []
        self._ops_total = 0
        self._errors = 0
        self._retries = 0
        self._retry_success = 0
        self._active_conns = 0
        self._last_ops = 0
        self._last_time = time.monotonic()
        self._process = psutil.Process(os.getpid())
        self._process.cpu_percent()

    def record_op(self, latency_us: int) -> None:
        with self._lock:
            self._ops_total += 1
            bisect.insort(self._latencies, min(latency_us, 60_000_000))

    def record_error(self) -> None:
        with self._lock:
            self._errors += 1

    def record_retry(self) -> None:
        with self._lock:
            self._retries += 1

    def record_retry_success(self) -> None:
        with self._lock:
            self._retry_success += 1

    def set_active_conns(self, n: int) -> None:
        with self._lock:
            self._active_conns = n

    def _percentile(self, data: list[int], p: float) -> float:
        if not data:
            return 0.0
        idx = int(len(data) * p / 100.0)
        idx = min(idx, len(data) - 1)
        return float(data[idx])

    def snapshot(self, run_id: str, mode: str, workload: str) -> MetricSnapshot:
        now = time.monotonic()
        now_ms = int(time.time() * 1000)

        with self._lock:
            current_ops = self._ops_total
            elapsed = now - self._last_time
            ops_per_sec = (current_ops - self._last_ops) / elapsed if elapsed > 0 else 0.0

            p50 = self._percentile(self._latencies, 50.0)
            p95 = self._percentile(self._latencies, 95.0)
            p99 = self._percentile(self._latencies, 99.0)
            max_val = float(self._latencies[-1]) if self._latencies else 0.0
            self._latencies.clear()

            self._last_ops = current_ops
            self._last_time = now

            errors = self._errors
            retries = self._retries
            retry_success = self._retry_success
            active_conns = self._active_conns

        cpu = self._process.cpu_percent()
        mem = self._process.memory_info().rss / (1024 * 1024)

        return MetricSnapshot(
            ts=now_ms,
            run_id=run_id,
            language="python",
            mode=mode,
            workload=workload,
            ops_total=current_ops,
            ops_per_sec=round(ops_per_sec, 2),
            latency_p50_us=p50,
            latency_p95_us=p95,
            latency_p99_us=p99,
            latency_max_us=max_val,
            errors=errors,
            retries=retries,
            retry_success=retry_success,
            circuit_breaker_open=False,
            active_connections=active_conns,
            cpu_percent=round(cpu, 2),
            mem_rss_mb=round(mem, 2),
        )
