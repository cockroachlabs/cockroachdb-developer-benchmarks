import abc
import os

from psycopg_pool import ConnectionPool

from agent.metrics import MetricsRecorder
from agent.protocol import WorkloadConfig


class WorkloadRunner(abc.ABC):
    def __init__(self, mode: str, recorder: MetricsRecorder) -> None:
        self.mode = mode
        self.recorder = recorder
        self.pool: ConnectionPool | None = None
        self._running = False
        self._db_url: str = ""

    def _resolve_db_url(self, config: WorkloadConfig) -> str:
        return config.db_url or os.getenv("DB_URL", "postgresql://root@localhost:26257/benchmark?sslmode=disable")

    def _make_pool(self, config: WorkloadConfig) -> ConnectionPool:
        db_url = self._resolve_db_url(config)
        self._db_url = db_url
        min_size = max(2, config.concurrency)
        max_size = min_size * 2

        if self.mode == "library":
            kwargs = {
                "conninfo": db_url,
                "min_size": min_size,
                "max_size": max_size,
                "kwargs": {
                    "autocommit": False,
                    "prepare_threshold": 5,
                    "options": "-c default_transaction_isolation=serializable",
                },
            }
        else:
            kwargs = {
                "conninfo": db_url,
                "min_size": min_size,
                "max_size": max_size,
            }

        pool = ConnectionPool(**kwargs, open=True)
        return pool

    def _get_sqla_url(self) -> str:
        url = self._db_url
        if url.startswith("postgresql://"):
            url = "cockroachdb://" + url[len("postgresql://"):]
        elif url.startswith("postgres://"):
            url = "cockroachdb://" + url[len("postgres://"):]
        return url

    @abc.abstractmethod
    def start(self, config: WorkloadConfig) -> None: ...

    def stop(self) -> None:
        self._running = False
        if self.pool:
            self.pool.close()
            self.pool = None

    @property
    def is_running(self) -> bool:
        return self._running
