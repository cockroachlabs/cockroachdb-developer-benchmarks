.PHONY: start stop restart dev topo-up topo-down agents-up agents-down agents-build \
       bench bench-all bench-chaos report monitoring-up monitoring-down \
       docker-stop docker-remove docker-status test clean help

DOCKER_LABEL := com.cockroachlabs.benchmark=true

CONTROLLER_PORT   ?= 9495
DASHBOARD_PORT    ?= 3000
PROMETHEUS_PORT   ?= 9090
GRAFANA_PORT      ?= 3001
TOPOLOGY          ?= single
DB_URL          ?=
WORKLOAD        ?= crud
AGENTS          ?= java,go,python,dotnet
CHAOS_MODE      ?= gauntlet
DURATION        ?= 60
CONCURRENCY     ?= 16
PIDS_DIR        := .pids
DEPLOY_DIR      := deploy
RESULTS_DIR     := results
COMPOSE         := $(shell command -v docker-compose 2>/dev/null || echo "$(COMPOSE)")

$(shell mkdir -p $(PIDS_DIR) $(RESULTS_DIR))

help:
	@echo "CockroachDB Developer Benchmarks"
	@echo "================================"
	@echo ""
	@echo "Lifecycle:"
	@echo "  make start                      Start controller + dashboard (default ports)"
	@echo "  make start CONTROLLER_PORT=9090  Custom controller port"
	@echo "  make start DASHBOARD_PORT=3001   Custom dashboard port"
	@echo "  make start DB_URL=postgresql://.. Use remote CockroachDB"
	@echo "  make stop                        Stop controller + dashboard"
	@echo "  make restart                     Stop + start"
	@echo "  make dev                         Dev mode with hot reload"
	@echo "  make dev PORT=5679               Dev mode with custom controller port"
	@echo ""
	@echo "Topology (CockroachDB):"
	@echo "  make topo-up                     Start single node (default)"
	@echo "  make topo-up TOPOLOGY=cluster    Start 3-node cluster"
	@echo "  make topo-up TOPOLOGY=multi      Start 9-node multi-region"
	@echo "  make topo-down                   Tear down CockroachDB"
	@echo ""
	@echo "Agents:"
	@echo "  make agents-up                   Build & start all 8 agent containers"
	@echo "  make agents-up AGENTS=go,python  Start specific language agents"
	@echo "  make agents-down                 Stop agent containers"
	@echo "  make agents-build                Rebuild agent Docker images"
	@echo ""
	@echo "Benchmarks:"
	@echo "  make bench WORKLOAD=crud         Run single workload"
	@echo "  make bench-all                   Run all 6 workloads sequentially"
	@echo "  make bench-chaos                 All workloads + chaos gauntlet (9-node)"
	@echo "  make report                      Download HTML report for latest run"
	@echo ""
	@echo "Docker:"
	@echo "  make docker-status               Show all benchmark containers and images"
	@echo "  make docker-stop                 Stop all benchmark containers"
	@echo "  make docker-remove               Remove all benchmark containers, images, volumes"
	@echo ""
	@echo "Maintenance:"
	@echo "  make test                        Run all agent unit tests"
	@echo "  make clean                       Remove builds, deps, data, and Docker resources"

# --- Lifecycle ---

start: _build-controller _build-dashboard
	@echo "Starting controller on port $(CONTROLLER_PORT)..."
	@cd controller && go build -o ../$(PIDS_DIR)/controller ./cmd/server
	@$(PIDS_DIR)/controller \
		--port=$(CONTROLLER_PORT) \
		--dashboard-dir=dashboard/dist \
		$(if $(DB_URL),--db-url="$(DB_URL)",) \
		--topology=$(TOPOLOGY) \
		& echo $$! > $(PIDS_DIR)/controller.pid
	@echo "Starting dashboard on port $(DASHBOARD_PORT)..."
	@cd dashboard && npx vite preview --port $(DASHBOARD_PORT) --host \
		& echo $$! > ../$(PIDS_DIR)/dashboard.pid
	@echo ""
	@echo "Controller: http://localhost:$(CONTROLLER_PORT)"
	@echo "Dashboard:  http://localhost:$(DASHBOARD_PORT)"
	@echo "CockroachDB UI: http://localhost:8080 (if local topology)"

stop:
	@echo "Stopping services..."
	@if [ -f $(PIDS_DIR)/controller.pid ]; then \
		kill $$(cat $(PIDS_DIR)/controller.pid) 2>/dev/null; \
		rm -f $(PIDS_DIR)/controller.pid; \
		echo "  Controller stopped"; \
	fi
	@if [ -f $(PIDS_DIR)/dashboard.pid ]; then \
		kill $$(cat $(PIDS_DIR)/dashboard.pid) 2>/dev/null; \
		rm -f $(PIDS_DIR)/dashboard.pid; \
		echo "  Dashboard stopped"; \
	fi

restart: stop start

dev: _ensure-deps
	@echo "Starting dev mode..."
	@cd controller && go run ./cmd/server \
		--port=$(if $(PORT),$(PORT),$(CONTROLLER_PORT)) \
		--topology=$(TOPOLOGY) \
		$(if $(DB_URL),--db-url="$(DB_URL)",) \
		& echo $$! > ../$(PIDS_DIR)/controller.pid
	@cd dashboard && npx vite --port $(DASHBOARD_PORT) --host \
		& echo $$! > ../$(PIDS_DIR)/dashboard.pid
	@echo ""
	@echo "Controller: http://localhost:$(if $(PORT),$(PORT),$(CONTROLLER_PORT))"
	@echo "Dashboard:  http://localhost:$(DASHBOARD_PORT) (hot reload)"

# --- Build helpers ---

_build-controller:
	@echo "Building controller..."
	@cd controller && go build -o ../$(PIDS_DIR)/controller ./cmd/server

_build-dashboard:
	@echo "Building dashboard..."
	@cd dashboard && npm install && npx vite build

_ensure-deps:
	@cd controller && go build ./cmd/server 2>/dev/null || (echo "Building controller..." && go build ./cmd/server)
	@test -d dashboard/node_modules || (echo "Installing dashboard deps..." && cd dashboard && npm install)

# --- Topology ---

topo-up:
	@echo "Starting CockroachDB ($(TOPOLOGY))..."
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.$(TOPOLOGY).yml \
		-p crdb-bench up -d
	@echo "Waiting for CockroachDB to be ready..."
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.$(TOPOLOGY).yml \
		-p crdb-bench exec -T crdb-1 \
		/cockroach/cockroach sql --insecure -e "SELECT 1" > /dev/null 2>&1 \
		&& echo "CockroachDB is ready" \
		|| echo "Waiting... (run 'make topo-up' again if init hasn't completed)"

topo-down:
	@echo "Tearing down CockroachDB..."
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.single.yml -p crdb-bench down -v 2>/dev/null || true
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.cluster.yml -p crdb-bench down -v 2>/dev/null || true
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.multi.yml -p crdb-bench down -v 2>/dev/null || true
	@echo "Done"

# --- Agents ---

agents-build: _copy-libs
	@echo "Building agent images..."
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.agents.yml build

_copy-libs:
	@echo "Copying libraries into agent build contexts..."
	@rsync -a --delete --exclude='.git' --exclude='node_modules' --exclude='bin' --exclude='obj' --exclude='build' --exclude='.gradle' \
		../cockroachdb-go-library/ agents/go/cockroachdb-go-library/
	@rsync -a --delete --exclude='.git' --exclude='node_modules' --exclude='build' --exclude='.gradle' \
		../cockroachdb-spring-boot-starter/ agents/java/cockroachdb-spring-boot-starter/
	@rsync -a --delete --exclude='.git' --exclude='node_modules' --exclude='__pycache__' --exclude='.venv' --exclude='*.egg-info' \
		../cockroachdb-python-library/ agents/python/cockroachdb-python-library/
	@rsync -a --delete --exclude='.git' --exclude='node_modules' --exclude='bin' --exclude='obj' \
		--exclude='tests' --exclude='*.Tests*' --exclude='*.IntegrationTests*' \
		--exclude='*EntityFramework*' --exclude='*Demo*' --exclude='*Testing*' \
		../cockroachdb-dotnet-library/ agents/dotnet/cockroachdb-dotnet-library/

agents-up: agents-build
	@echo "Starting agents..."
	@if [ "$(AGENTS)" = "java,go,python,dotnet" ]; then \
		$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.agents.yml up -d; \
	else \
		for lang in $$(echo $(AGENTS) | tr ',' ' '); do \
			$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.agents.yml \
				up -d agent-$$lang-lib agent-$$lang-vanilla; \
		done; \
	fi

agents-down:
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.agents.yml down
	@echo "Agents stopped"

# --- Benchmarks ---

bench:
	@echo "Running benchmark: $(WORKLOAD)"
	@curl -s -X POST http://localhost:$(CONTROLLER_PORT)/api/benchmark/start \
		-H "Content-Type: application/json" \
		-d '{"workloads":["$(WORKLOAD)"],"topology":"$(TOPOLOGY)","duration":$(DURATION),"concurrency":$(CONCURRENCY),"chaos_mode":"none"}' \
		| python3 -m json.tool 2>/dev/null || echo '{"error":"controller not running"}'

bench-all:
	@echo "Running all benchmarks sequentially..."
	@curl -s -X POST http://localhost:$(CONTROLLER_PORT)/api/benchmark/start \
		-H "Content-Type: application/json" \
		-d '{"workloads":["crud","retry_storm","mixed_rw","follower_reads","cdc","vector_search"],"topology":"$(TOPOLOGY)","duration":$(DURATION),"concurrency":$(CONCURRENCY),"chaos_mode":"none"}' \
		| python3 -m json.tool 2>/dev/null || echo '{"error":"controller not running"}'

bench-chaos:
	@echo "Running chaos benchmark (9-node required)..."
	@curl -s -X POST http://localhost:$(CONTROLLER_PORT)/api/benchmark/start \
		-H "Content-Type: application/json" \
		-d '{"workloads":["crud","retry_storm","mixed_rw","follower_reads","cdc","vector_search"],"topology":"multi","duration":90,"concurrency":$(CONCURRENCY),"chaos_mode":"$(CHAOS_MODE)"}' \
		| python3 -m json.tool 2>/dev/null || echo '{"error":"controller not running"}'

# --- Report ---

report:
	@echo "Generating HTML report for latest run..."
	@curl -s http://localhost:$(CONTROLLER_PORT)/api/runs/latest/report \
		-o results/report-$$(date +%Y%m%d-%H%M%S).html
	@echo "Report saved to results/"

# --- Monitoring ---

monitoring-up:
	@echo "Starting Prometheus + Grafana..."
	@PROMETHEUS_PORT=$(PROMETHEUS_PORT) GRAFANA_PORT=$(GRAFANA_PORT) \
		$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.monitoring.yml -p crdb-monitoring up -d
	@echo ""
	@echo "Prometheus: http://localhost:$(PROMETHEUS_PORT)"
	@echo "Grafana:    http://localhost:$(GRAFANA_PORT) (admin/benchmark)"
	@echo "Dashboard auto-provisioned: CockroachDB Developer Benchmarks"

monitoring-down:
	@echo "Stopping Prometheus + Grafana..."
	@$(COMPOSE) -f $(DEPLOY_DIR)/docker-compose.monitoring.yml -p crdb-monitoring down -v

# --- Testing ---

test:
	@echo "Running controller tests..."
	@cd controller && go test ./... -v -count=1
	@echo "Running Go agent tests..."
	@cd agents/go && go test ./... -v -count=1
	@echo "Running dashboard tests..."
	@cd dashboard && npx vitest run

# --- Docker Management ---

docker-status:
	@echo "=== CockroachDB Benchmark Containers ==="
	@docker ps -a --filter "name=crdb-bench" --filter "name=deploy-agent" --filter "name=crdb-monitoring" \
		--format "table {{.Names}}\t{{.Status}}\t{{.Ports}}" 2>/dev/null || true
	@echo ""
	@echo "=== CockroachDB Benchmark Images ==="
	@docker images --filter "reference=deploy-agent-*" --filter "reference=crdb-bench*" \
		--format "table {{.Repository}}\t{{.Tag}}\t{{.Size}}" 2>/dev/null || true

docker-stop:
	@echo "Stopping all benchmark containers..."
	@docker ps -q --filter "name=crdb-bench" --filter "name=deploy-agent" --filter "name=crdb-monitoring" \
		| xargs -r docker stop 2>/dev/null || true
	@echo "All benchmark containers stopped"

docker-remove: docker-stop
	@echo "Removing benchmark containers..."
	@docker ps -aq --filter "name=crdb-bench" --filter "name=deploy-agent" --filter "name=crdb-monitoring" \
		| xargs -r docker rm -f 2>/dev/null || true
	@echo "Removing benchmark images..."
	@docker images -q --filter "reference=deploy-agent-*" \
		| xargs -r docker rmi -f 2>/dev/null || true
	@echo "Removing benchmark volumes..."
	@docker volume ls -q --filter "name=crdb-bench" --filter "name=crdb-monitoring" \
		| xargs -r docker volume rm -f 2>/dev/null || true
	@echo "Removing benchmark networks..."
	@docker network ls -q --filter "name=crdb-bench" --filter "name=crdb-monitoring" \
		| xargs -r docker network rm 2>/dev/null || true
	@echo "All benchmark Docker resources removed"

# --- Cleanup ---

clean: stop topo-down agents-down
	@echo "Cleaning up..."
	@rm -rf $(PIDS_DIR)/*
	@rm -rf results/*.json results/*.html
	@rm -f benchmark.db data/benchmarks.db
	@rm -rf data/results
	@rm -f controller-bin controller/server
	@rm -rf dashboard/dist dashboard/node_modules
	@rm -rf dashboard/package-lock.json
	@rm -rf controller/tmp
	@rm -rf agents/go/cockroachdb-go-library
	@rm -rf agents/java/cockroachdb-spring-boot-starter
	@rm -rf agents/java/build agents/java/.gradle
	@rm -rf agents/python/cockroachdb-python-library
	@rm -rf agents/dotnet/cockroachdb-dotnet-library
	@rm -rf agents/dotnet/bin agents/dotnet/obj
	@rm -rf scenarios/custom/*.json
	@echo "Clean complete — run 'make start' or 'make dev' to rebuild"
