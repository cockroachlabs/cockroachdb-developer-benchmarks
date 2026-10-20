FROM golang:latest AS builder

WORKDIR /app

COPY cockroachdb-go-library/ /cockroachdb-go-library/

COPY go.mod go.sum ./
RUN sed -i 's|/Users/felipeg/Progs/AI/cockroachdb-go-library|/cockroachdb-go-library|g' go.mod
RUN go mod download

COPY . .
RUN sed -i 's|/Users/felipeg/Progs/AI/cockroachdb-go-library|/cockroachdb-go-library|g' go.mod
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /agent ./cmd/agent

FROM alpine:3.19

RUN apk add --no-cache ca-certificates

COPY --from=builder /agent /agent

ENV AGENT_MODE=vanilla
ENV AGENT_PORT=7004
ENV DB_URL=postgresql://root@crdb-1:26257/benchmark?sslmode=disable

EXPOSE ${AGENT_PORT}

ENTRYPOINT ["/agent"]
