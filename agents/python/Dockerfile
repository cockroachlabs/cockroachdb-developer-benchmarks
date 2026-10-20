FROM python:3.12-slim AS builder

WORKDIR /build

COPY pyproject.toml .
RUN pip install --no-cache-dir .

COPY cockroachdb-python-library/packages/ /lib/packages/

RUN pip install --no-cache-dir \
    /lib/packages/cockroachdb \
    /lib/packages/cockroachdb-core \
    /lib/packages/cockroachdb-cdc \
    /lib/packages/cockroachdb-ai \
    /lib/packages/cockroachdb-sqlalchemy 2>/dev/null || \
    pip install --no-cache-dir /lib/packages/cockroachdb /lib/packages/cockroachdb-core 2>/dev/null || true

COPY agent/ agent/

FROM python:3.12-slim

WORKDIR /app

COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY --from=builder /usr/local/bin /usr/local/bin
COPY agent/ agent/

ENV PYTHONUNBUFFERED=1
ENV AGENT_MODE=vanilla
ENV AGENT_PORT=7006

EXPOSE 7005 7006

CMD sh -c "python -m uvicorn agent.main:app --host 0.0.0.0 --port $AGENT_PORT"
