import random
import time
from concurrent.futures import ThreadPoolExecutor

import psycopg
import psycopg.errors
from sqlalchemy import text

from agent.metrics import MetricsRecorder
from agent.protocol import WorkloadConfig
from agent.workloads.base import WorkloadRunner

MAX_RETRIES = 10


class RetryStormWorkload(WorkloadRunner):
    def __init__(self, mode: str, recorder: MetricsRecorder) -> None:
        super().__init__(mode, recorder)
        self._crdb = None

    def start(self, config: WorkloadConfig) -> None:
        self.pool = self._make_pool(config)
        self._running = True
        hot_rows = config.hot_rows if config.hot_rows > 0 else 10
        concurrency = max(config.concurrency, 32)
        self.recorder.set_active_conns(concurrency)

        with self.pool.connection() as conn:
            with conn.cursor() as cur:
                cur.execute("SELECT id FROM bench_accounts LIMIT %s", (hot_rows,))
                self._hot_ids = [str(row[0]) for row in cur.fetchall()]

        if not self._hot_ids:
            return

        if self.mode == "library":
            from cockroachdb.sqla import create_cockroachdb_engine, CockroachDBSession
            engine = create_cockroachdb_engine(self._get_sqla_url())
            self._crdb = CockroachDBSession(engine)

        with ThreadPoolExecutor(max_workers=concurrency) as executor:
            futures = [executor.submit(self._worker, config) for _ in range(concurrency)]
            for f in futures:
                f.result()

    def _worker(self, config: WorkloadConfig) -> None:
        deadline = time.monotonic() + config.duration_secs
        while self._running and time.monotonic() < deadline:
            account_id = random.choice(self._hot_ids)
            amount = round(random.uniform(-100, 100), 2)
            if self.mode == "library":
                self._do_library_retry(account_id, amount)
            else:
                self._do_vanilla_retry(account_id, amount)

    def _do_library_retry(self, account_id: str, amount: float) -> None:
        start = time.monotonic()
        try:
            self._crdb.execute_with_retry(lambda session: session.execute(
                text("UPDATE bench_accounts SET balance = balance + :amount, updated_at = now() WHERE id = :id"),
                {"amount": amount, "id": account_id},
            ))
            self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
        except Exception:
            self.recorder.record_error()

    def _do_vanilla_retry(self, account_id: str, amount: float) -> None:
        start = time.monotonic()
        for attempt in range(MAX_RETRIES):
            try:
                with self.pool.connection() as conn:
                    with conn.cursor() as cur:
                        cur.execute(
                            "UPDATE bench_accounts SET balance = balance + %s, updated_at = now() WHERE id = %s",
                            (amount, account_id),
                        )
                    conn.commit()
                if attempt > 0:
                    self.recorder.record_retry_success()
                self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
                return
            except psycopg.errors.SerializationFailure:
                self.recorder.record_retry()
                time.sleep(0.01 * (attempt + 1))
            except Exception:
                self.recorder.record_error()
                return
        self.recorder.record_error()
