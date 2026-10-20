import json
import random
import time
import uuid
from concurrent.futures import ThreadPoolExecutor

import psycopg
import numpy as np

from agent.metrics import MetricsRecorder
from agent.protocol import WorkloadConfig
from agent.workloads.base import WorkloadRunner

VECTOR_DIM = 1536
TOPICS = ["database", "distributed", "consensus", "replication", "transaction",
          "isolation", "serializable", "mvcc", "raft", "leaseholder"]


class VectorSearchWorkload(WorkloadRunner):
    def __init__(self, mode: str, recorder: MetricsRecorder) -> None:
        super().__init__(mode, recorder)
        self._template = None
        self._vst = None

    def start(self, config: WorkloadConfig) -> None:
        self.pool = self._make_pool(config)
        self._running = True
        self.recorder.set_active_conns(config.concurrency)

        if self.mode == "library":
            from cockroachdb.core import CockroachDBTemplate
            from cockroachdb.ai import VectorSearchTemplate
            self._template = CockroachDBTemplate(self._db_url)
            self._vst = VectorSearchTemplate(self._db_url)

        self._seed_documents(50)

        with ThreadPoolExecutor(max_workers=config.concurrency) as executor:
            futures = [executor.submit(self._worker, config) for _ in range(config.concurrency)]
            for f in futures:
                f.result()

    def _random_vector(self) -> list[float]:
        vec = np.random.randn(VECTOR_DIM).astype(np.float32)
        vec = vec / np.linalg.norm(vec)
        return vec.tolist()

    def _vector_to_sql(self, vec: list[float]) -> str:
        return "[" + ",".join(f"{v:.6f}" for v in vec) + "]"

    def _seed_documents(self, count: int) -> None:
        try:
            if self.mode == "library":
                for _ in range(count):
                    topic = random.choice(TOPICS)
                    content = f"Document about {topic} in CockroachDB - {uuid.uuid4().hex[:8]}"
                    vec = self._random_vector()
                    metadata = json.dumps({"topic": topic, "source": "benchmark"})
                    vec_sql = self._vector_to_sql(vec)
                    row_id = str(uuid.uuid4())

                    def fn(conn: psycopg.Connection, rid=row_id, c=content, v=vec_sql, m=metadata) -> None:
                        conn.execute(
                            "INSERT INTO bench_documents (id, content, embedding, metadata) "
                            "VALUES (%s, %s, %s::VECTOR(1536), %s)",
                            (rid, c, v, m),
                        )

                    self._template.execute(fn)
            else:
                with self.pool.connection() as conn:
                    with conn.cursor() as cur:
                        for _ in range(count):
                            topic = random.choice(TOPICS)
                            content = f"Document about {topic} in CockroachDB - {uuid.uuid4().hex[:8]}"
                            vec = self._random_vector()
                            metadata = json.dumps({"topic": topic, "source": "benchmark"})
                            cur.execute(
                                "INSERT INTO bench_documents (id, content, embedding, metadata) "
                                "VALUES (%s, %s, %s::VECTOR(1536), %s)",
                                (str(uuid.uuid4()), content, self._vector_to_sql(vec), metadata),
                            )
                    conn.commit()
        except Exception:
            self.recorder.record_error()

    def _worker(self, config: WorkloadConfig) -> None:
        deadline = time.monotonic() + config.duration_secs
        while self._running and time.monotonic() < deadline:
            try:
                if random.random() < 0.3:
                    self._do_insert()
                else:
                    self._do_search()
            except Exception:
                self.recorder.record_error()

    def _do_insert(self) -> None:
        start = time.monotonic()
        topic = random.choice(TOPICS)
        content = f"Document about {topic} in CockroachDB - {uuid.uuid4().hex[:8]}"
        vec = self._random_vector()
        metadata = json.dumps({"topic": topic, "source": "benchmark"})
        vec_sql = self._vector_to_sql(vec)
        row_id = str(uuid.uuid4())

        if self.mode == "library":
            def fn(conn: psycopg.Connection) -> None:
                conn.execute(
                    "INSERT INTO bench_documents (id, content, embedding, metadata) "
                    "VALUES (%s, %s, %s::VECTOR(1536), %s)",
                    (row_id, content, vec_sql, metadata),
                )

            self._template.execute(fn)
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "INSERT INTO bench_documents (id, content, embedding, metadata) "
                        "VALUES (%s, %s, %s::VECTOR(1536), %s)",
                        (row_id, content, vec_sql, metadata),
                    )
                conn.commit()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))

    def _do_search(self) -> None:
        start = time.monotonic()
        query_vec = self._random_vector()

        if self.mode == "library":
            from cockroachdb.types import DistanceMetric
            self._vst.search(
                table="bench_documents",
                vector_column="embedding",
                query_vector=query_vec,
                metric=DistanceMetric.COSINE,
                limit=10,
                columns=["id", "content"],
            )
        else:
            with self.pool.connection() as conn:
                with conn.cursor() as cur:
                    cur.execute(
                        "SELECT id, content FROM bench_documents "
                        "ORDER BY embedding <=> %s::VECTOR(1536) LIMIT 10",
                        (self._vector_to_sql(query_vec),),
                    )
                    cur.fetchall()
        self.recorder.record_op(int((time.monotonic() - start) * 1_000_000))
