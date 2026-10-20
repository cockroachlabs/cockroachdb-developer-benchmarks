import random
import time
from concurrent.futures import ThreadPoolExecutor

import psycopg

from agent.metrics import MetricsRecorder
from agent.protocol import WorkloadConfig
from agent.workloads.base import WorkloadRunner


class FollowerReadsWorkload(WorkloadRunner):
    def __init__(self, mode: str, recorder: MetricsRecorder) -> None:
        super().__init__(mode, recorder)
        self._template = None

    def start(self, config: WorkloadConfig) -> None:
        self.pool = self._make_pool(config)
        self._running = True
        self.recorder.set_active_conns(config.concurrency)

        if self.mode == "library":
            from cockroachdb.core import CockroachDBTemplate
            self._template = CockroachDBTemplate(self._db_url)

        with ThreadPoolExecutor(max_workers=config.concurrency) as executor:
            futures = [executor.submit(self._worker, config) for _ in range(config.concurrency)]
            for f in futures:
                f.result()

    def _worker(self, config: WorkloadConfig) -> None:
        deadline = time.monotonic() + config.duration_secs
        while self._running and time.monotonic() < deadline:
            try:
                if random.random() < 0.9:
                    self._do_follower_read()
                else:
                    self._do_write()
            except Exception:
                self.recorder.record_error()

    def _do_follower_read(self) -> None:
        start = time.monotonic()
        region = random.choice(["us-east", "us-west", "eu-west"])
        if self.mode == "library":
            with self._template.follower_read():
                def fn(conn: psycopg.Connection) -> list:
                    cur = conn.execute(
                        "SELECT id, balance, name, region FROM bench_accounts "
                        "WHERE region = %s ORDER BY balance DESC LIMIT 50",
                        (region,),
                    )
                    return cur.fetchall()

                self._template.execute_read_committed(fn)
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "SELECT id, balance, name, region FROM bench_accounts "
                        "AS OF SYSTEM TIME '-5s' "
                        "WHERE region = %s ORDER BY balance DESC LIMIT 50",
                        (region,),
                    )
                    cur.fetchall()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))

    def _do_write(self) -> None:
        start = time.monotonic()
        amount = round(random.uniform(-100, 100), 2)
        if self.mode == "library":
            def fn(conn: psycopg.Connection) -> None:
                conn.execute(
                    "UPDATE bench_accounts SET balance = balance + %s, updated_at = now() "
                    "WHERE id = (SELECT id FROM bench_accounts ORDER BY random() LIMIT 1)",
                    (amount,),
                )

            self._template.execute(fn)
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "UPDATE bench_accounts SET balance = balance + %s, updated_at = now() "
                        "WHERE id = (SELECT id FROM bench_accounts ORDER BY random() LIMIT 1)",
                        (amount,),
                    )
                conn.commit()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
