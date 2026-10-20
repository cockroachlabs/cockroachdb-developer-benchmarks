import asyncio
import json
import os
import threading
import time
from contextlib import asynccontextmanager

from fastapi import FastAPI, HTTPException, WebSocket, WebSocketDisconnect

from agent.metrics import MetricsRecorder
from agent.protocol import (
    AgentInfo,
    HealthResponse,
    MetricSnapshot,
    StopResponse,
    WorkloadRequest,
    WorkloadResponse,
)
from agent.workloads.factory import get_workload

MODE = os.getenv("AGENT_MODE", "vanilla")
PORT = int(os.getenv("AGENT_PORT", "7005" if MODE == "library" else "7006"))

_lock = threading.Lock()
_recorder: MetricsRecorder | None = None
_cancel_event: threading.Event | None = None
_run_id: str = ""
_workload_type: str = ""
_running: bool = False
_ws_clients: list[asyncio.Queue[MetricSnapshot]] = []
_ws_lock = threading.Lock()


@asynccontextmanager
async def lifespan(app: FastAPI):
    yield

app = FastAPI(lifespan=lifespan)


@app.post("/workload")
async def start_workload(req: WorkloadRequest) -> WorkloadResponse:
    global _recorder, _cancel_event, _run_id, _workload_type, _running

    with _lock:
        if _running:
            raise HTTPException(status_code=409, detail="workload already running")

        recorder = MetricsRecorder()
        mode = req.mode or MODE
        runner = get_workload(req.type, mode, recorder)
        if runner is None:
            raise HTTPException(status_code=400, detail=f"unknown workload: {req.type}")

        run_id = f"python-{mode}-{req.type}-{int(time.time() * 1000)}"
        cancel_event = threading.Event()
        _recorder = recorder
        _cancel_event = cancel_event
        _run_id = run_id
        _workload_type = req.type
        _running = True

    if not req.config.db_url:
        req.config.db_url = os.getenv("DB_URL", "postgresql://root@localhost:26257/benchmark?sslmode=disable")

    def _run() -> None:
        global _running
        try:
            runner.start(req.config)
        except Exception:
            pass
        finally:
            with _lock:
                _running = False

    def _emit() -> None:
        while not cancel_event.is_set():
            cancel_event.wait(1.0)
            if cancel_event.is_set():
                break
            with _lock:
                if not _running or _recorder is None:
                    break
                snap = _recorder.snapshot(_run_id, mode, _workload_type)
            with _ws_lock:
                for q in _ws_clients:
                    try:
                        q.put_nowait(snap)
                    except asyncio.QueueFull:
                        pass

    threading.Thread(target=_run, daemon=True).start()
    threading.Thread(target=_emit, daemon=True).start()

    return WorkloadResponse(run_id=run_id, status="started")


@app.post("/stop")
async def stop_workload() -> StopResponse:
    global _running, _cancel_event
    with _lock:
        if not _running:
            return StopResponse(status="idle")
        _running = False
        if _cancel_event:
            _cancel_event.set()
    return StopResponse(status="stopped")


@app.get("/health")
async def health() -> HealthResponse:
    return HealthResponse(status="ok", language="python", mode=MODE)


@app.get("/info")
async def info() -> AgentInfo:
    return AgentInfo(language="python", library_version="0.1.0", driver="psycopg3")


@app.websocket("/metrics")
async def metrics_ws(ws: WebSocket) -> None:
    await ws.accept()
    queue: asyncio.Queue[MetricSnapshot] = asyncio.Queue(maxsize=64)
    with _ws_lock:
        _ws_clients.append(queue)
    try:
        while True:
            snap = await queue.get()
            await ws.send_text(json.dumps(snap.model_dump()))
    except WebSocketDisconnect:
        pass
    except Exception:
        pass
    finally:
        with _ws_lock:
            if queue in _ws_clients:
                _ws_clients.remove(queue)


if __name__ == "__main__":
    import uvicorn
    uvicorn.run("agent.main:app", host="0.0.0.0", port=PORT)
