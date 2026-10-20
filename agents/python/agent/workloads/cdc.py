import json
import random
import threading
import time
import uuid

import psycopg

from agent.metrics import MetricsRecorder
from agent.protocol import WorkloadConfig
from agent.workloads.base import WorkloadRunner


class CDCWorkload(WorkloadRunner):
    def __init__(self, mode: str, recorder: MetricsRecorder) -> None:
        super().__init__(mode, recorder)
        self._writer_running = False
        self._template = None

    def start(self, config: WorkloadConfig) -> None:
        self.pool = self._make_pool(config)
        self._running = True
        self._writer_running = True
        self.recorder.set_active_conns(config.concurrency)

        if self.mode == "library":
            from cockroachdb.core import CockroachDBTemplate
            self._template = CockroachDBTemplate(self._db_url)

        writer_thread = threading.Thread(target=self._writer_loop, args=(config,), daemon=True)
        writer_thread.start()

        consumer_thread = threading.Thread(target=self._consumer_loop, args=(config,), daemon=True)
        consumer_thread.start()

        deadline = time.monotonic() + config.duration_secs
        while self._running and time.monotonic() < deadline:
            time.sleep(0.5)

        self._writer_running = False
        writer_thread.join(timeout=5)
        consumer_thread.join(timeout=5)

    def _writer_loop(self, config: WorkloadConfig) -> None:
        deadline = time.monotonic() + config.duration_secs
        while self._writer_running and time.monotonic() < deadline:
            try:
                start = time.monotonic()
                if self.mode == "library":
                    row_id = str(uuid.uuid4())
                    balance = round(random.uniform(100, 10000), 2)
                    name = f"cdc-user-{random.randint(1, 999999)}"
                    region = random.choice(["us-east", "us-west", "eu-west"])

                    def fn(conn: psycopg.Connection) -> None:
                        conn.execute(
                            "INSERT INTO bench_accounts (id, balance, name, region) VALUES (%s, %s, %s, %s)",
                            (row_id, balance, name, region),
                        )

                    self._template.execute(fn)
                else:
                    with self.pool.connection() as conn:
                        with conn.cursor() as cur:
                            cur.execute(
                                "INSERT INTO bench_accounts (id, balance, name, region) VALUES (%s, %s, %s, %s)",
                                (str(uuid.uuid4()), round(random.uniform(100, 10000), 2),
                                 f"cdc-user-{random.randint(1, 999999)}",
                                 random.choice(["us-east", "us-west", "eu-west"])),
                            )
                        conn.commit()
                self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
                time.sleep(0.01)
            except Exception:
                self.recorder.record_error()

    def _consumer_loop(self, config: WorkloadConfig) -> None:
        deadline = time.monotonic() + config.duration_secs
        if self.mode == "library":
            self._consume_library(deadline)
        else:
            self._consume_vanilla(deadline)

    def _consume_library(self, deadline: float) -> None:
        from cockroachdb.cdc import ChangefeedHandlerRegistry, ChangefeedManager
        from cockroachdb.cdc.models import ChangefeedConfig
        from cockroachdb import changefeed_handler

        @changefeed_handler(table="bench_accounts", operations=["insert", "update"])
        def on_account_change(event) -> None:
            pass

        registry = ChangefeedHandlerRegistry()
        registry.register(on_account_change)

        try:
            with psycopg.connect(self._db_url, autocommit=True) as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "EXPERIMENTAL CHANGEFEED FOR bench_accounts WITH updated, resolved='5s'"
                    )
                    while self._running and time.monotonic() < deadline:
                        row = cur.fetchone()
                        if row is None:
                            break
                        start = time.monotonic()
                        payload = {"table": str(row[0]), "key": str(row[1]), "value": str(row[2]) if len(row) > 2 else ""}

                        def insert_event(c: psycopg.Connection) -> None:
                            c.execute(
                                "INSERT INTO bench_cdc_events (id, source_table, operation, payload, lag_ms) "
                                "VALUES (%s, %s, %s, %s, %s)",
                                (str(uuid.uuid4()), "bench_accounts", "changefeed",
                                 json.dumps(payload), 0),
                            )

                        self._template.execute(insert_event)
                        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
        except Exception:
            self.recorder.record_error()

    def _consume_vanilla(self, deadline: float) -> None:
        while self._running and time.monotonic() < deadline:
            try:
                start = time.monotonic()
                with self.pool.connection() as conn:
                    with conn.cursor() as cur:
                        cur.execute(
                            "SELECT id, balance, name, updated_at FROM bench_accounts "
                            "WHERE updated_at > now() - INTERVAL '2 seconds' LIMIT 50"
                        )
                        rows = cur.fetchall()
                        for row in rows:
                            payload = {"id": str(row[0]), "balance": str(row[1]), "name": str(row[2])}
                            with self.pool.connection() as wconn:
                                with wconn.cursor() as wcur:
                                    wcur.execute(
                                        "INSERT INTO bench_cdc_events (id, source_table, operation, payload, lag_ms) "
                                        "VALUES (%s, %s, %s, %s, %s)",
                                        (str(uuid.uuid4()), "bench_accounts", "poll",
                                         json.dumps(payload), 0),
                                    )
                                wconn.commit()
                self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
                time.sleep(0.5)
            except Exception:
                self.recorder.record_error()
                time.sleep(1)
