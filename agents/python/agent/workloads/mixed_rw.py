import random
import time
import uuid
from concurrent.futures import ThreadPoolExecutor

import psycopg

from agent.metrics import MetricsRecorder
from agent.protocol import WorkloadConfig
from agent.workloads.base import WorkloadRunner

STATUSES = ["pending", "completed", "cancelled", "refunded"]


class MixedRWWorkload(WorkloadRunner):
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
                if random.random() < 0.8:
                    self._do_read()
                else:
                    self._do_write()
            except Exception:
                self.recorder.record_error()

    def _do_read(self) -> None:
        start = time.monotonic()
        query = random.choice(["accounts", "orders", "join"])
        if self.mode == "library":
            def fn(conn: psycopg.Connection) -> list:
                if query == "accounts":
                    cur = conn.execute(
                        "SELECT id, balance, name, region FROM bench_accounts "
                        "WHERE region = %s ORDER BY balance DESC LIMIT 20",
                        (random.choice(["us-east", "us-west", "eu-west"]),),
                    )
                elif query == "orders":
                    cur = conn.execute(
                        "SELECT id, account_id, amount, status FROM bench_orders "
                        "ORDER BY ts DESC LIMIT 20"
                    )
                else:
                    cur = conn.execute(
                        "SELECT a.name, a.balance, o.amount, o.status "
                        "FROM bench_accounts a JOIN bench_orders o ON a.id = o.account_id "
                        "ORDER BY o.ts DESC LIMIT 20"
                    )
                return cur.fetchall()

            self._template.execute_read_committed(fn)
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    if query == "accounts":
                        cur.execute(
                            "SELECT id, balance, name, region FROM bench_accounts "
                            "WHERE region = %s ORDER BY balance DESC LIMIT 20",
                            (random.choice(["us-east", "us-west", "eu-west"]),),
                        )
                    elif query == "orders":
                        cur.execute(
                            "SELECT id, account_id, amount, status FROM bench_orders "
                            "ORDER BY ts DESC LIMIT 20"
                        )
                    else:
                        cur.execute(
                            "SELECT a.name, a.balance, o.amount, o.status "
                            "FROM bench_accounts a JOIN bench_orders o ON a.id = o.account_id "
                            "ORDER BY o.ts DESC LIMIT 20"
                        )
                    cur.fetchall()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))

    def _do_write(self) -> None:
        start = time.monotonic()
        order_id = str(uuid.uuid4())
        amount = round(random.uniform(10, 5000), 2)
        status = random.choice(STATUSES)
        debit = round(random.uniform(1, 100), 2)

        if self.mode == "library":
            def fn(conn: psycopg.Connection) -> None:
                cur = conn.execute("SELECT id FROM bench_accounts ORDER BY random() LIMIT 1")
                row = cur.fetchone()
                if row:
                    account_id = str(row[0])
                    conn.execute(
                        "INSERT INTO bench_orders (id, account_id, amount, status) "
                        "VALUES (%s, %s, %s, %s)",
                        (order_id, account_id, amount, status),
                    )
                    conn.execute(
                        "UPDATE bench_accounts SET balance = balance - %s, updated_at = now() WHERE id = %s",
                        (debit, account_id),
                    )

            self._template.execute(fn)
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute("SELECT id FROM bench_accounts ORDER BY random() LIMIT 1")
                    row = cur.fetchone()
                    if row:
                        account_id = str(row[0])
                        cur.execute(
                            "INSERT INTO bench_orders (id, account_id, amount, status) "
                            "VALUES (%s, %s, %s, %s)",
                            (order_id, account_id, amount, status),
                        )
                        cur.execute(
                            "UPDATE bench_accounts SET balance = balance - %s, updated_at = now() WHERE id = %s",
                            (debit, account_id),
                        )
                conn.commit()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
