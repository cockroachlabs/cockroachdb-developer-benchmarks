import random
import time
import uuid
from concurrent.futures import ThreadPoolExecutor
from decimal import Decimal

from sqlalchemy import text

from agent.metrics import MetricsRecorder
from agent.protocol import WorkloadConfig
from agent.workloads.base import WorkloadRunner

REGIONS = ["us-east", "us-west", "eu-west"]


class CRUDWorkload(WorkloadRunner):
    def __init__(self, mode: str, recorder: MetricsRecorder) -> None:
        super().__init__(mode, recorder)
        self._crdb = None

    def start(self, config: WorkloadConfig) -> None:
        self.pool = self._make_pool(config)
        self._running = True
        self.recorder.set_active_conns(config.concurrency)

        if self.mode == "library":
            from cockroachdb.sqla import create_cockroachdb_engine, CockroachDBSession
            engine = create_cockroachdb_engine(self._get_sqla_url())
            self._crdb = CockroachDBSession(engine)

        with ThreadPoolExecutor(max_workers=config.concurrency) as executor:
            futures = [executor.submit(self._worker, config) for _ in range(config.concurrency)]
            for f in futures:
                f.result()

    def _worker(self, config: WorkloadConfig) -> None:
        deadline = time.monotonic() + config.duration_secs
        while self._running and time.monotonic() < deadline:
            op = random.choice(["insert", "select", "update", "delete"])
            try:
                if op == "insert":
                    self._do_insert()
                elif op == "select":
                    self._do_select()
                elif op == "update":
                    self._do_update()
                else:
                    self._do_delete()
            except Exception:
                self.recorder.record_error()

    def _do_insert(self) -> None:
        start = time.monotonic()
        if self.mode == "library":
            row_id = str(uuid.uuid4())
            balance = round(random.uniform(100, 10000), 2)
            name = f"user-{random.randint(1, 999999)}"
            region = random.choice(REGIONS)

            self._crdb.execute_with_retry(lambda session: session.execute(
                text("INSERT INTO bench_accounts (id, balance, name, region) VALUES (:id, :balance, :name, :region)"),
                {"id": row_id, "balance": balance, "name": name, "region": region},
            ))
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "INSERT INTO bench_accounts (id, balance, name, region) VALUES (%s, %s, %s, %s)",
                        (str(uuid.uuid4()), round(random.uniform(100, 10000), 2),
                         f"user-{random.randint(1, 999999)}", random.choice(REGIONS)),
                    )
                conn.commit()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))

    def _do_select(self) -> None:
        start = time.monotonic()
        row_id = str(uuid.uuid4())
        if self.mode == "library":
            self._crdb.execute_with_retry(lambda session: session.execute(
                text("SELECT name, balance FROM bench_accounts WHERE id = :id"),
                {"id": row_id},
            ).fetchall())
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute("SELECT name, balance FROM bench_accounts WHERE id = %s", (row_id,))
                    cur.fetchall()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))

    def _do_update(self) -> None:
        start = time.monotonic()
        # balance is DECIMAL; bind a Decimal so CockroachDB does decimal+decimal
        # (a Python float binds as float8 and `decimal + float8` is unsupported).
        amount = Decimal(str(round(random.uniform(-50, 50), 2)))
        row_id = str(uuid.uuid4())
        if self.mode == "library":
            self._crdb.execute_with_retry(lambda session: session.execute(
                text("UPDATE bench_accounts SET balance = balance + :amount, updated_at = now() "
                     "WHERE id = :id"),
                {"amount": amount, "id": row_id},
            ))
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "UPDATE bench_accounts SET balance = balance + %s, updated_at = now() "
                        "WHERE id = %s",
                        (amount, row_id),
                    )
                conn.commit()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))

    def _do_delete(self) -> None:
        start = time.monotonic()
        row_id = str(uuid.uuid4())
        if self.mode == "library":
            self._crdb.execute_with_retry(lambda session: session.execute(
                text("DELETE FROM bench_accounts WHERE id = :id"),
                {"id": row_id},
            ))
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute("DELETE FROM bench_accounts WHERE id = %s", (row_id,))
                conn.commit()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
