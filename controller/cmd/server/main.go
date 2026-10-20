package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/api"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/chaos"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/collector"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/scheduler"
	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/topology"
)

func main() {
	port := flag.Int("port", 9495, "HTTP server port")
	topoFlag := flag.String("topology", "", "Initial topology to start (single, cluster, multi)")
	dbURL := flag.String("db-url", "", "Remote database URL (skip Docker topology)")
	dashDir := flag.String("dashboard-dir", "", "Path to dashboard/dist/ for static file serving")
	deployDir := flag.String("deploy-dir", "", "Path to deploy/ directory with docker-compose files")
	dataDir := flag.String("data-dir", "", "Path to data directory for SQLite and results")
	scenariosDir := flag.String("scenarios-dir", "", "Path to scenarios/ directory with JSON scenario files")
	flag.Parse()

	if *deployDir == "" {
		exe, _ := os.Executable()
		*deployDir = filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(exe))), "deploy")
	}

	if *dataDir == "" {
		*dataDir = filepath.Join(*deployDir, "..", "data")
	}

	if *scenariosDir == "" {
		exe, _ := os.Executable()
		*scenariosDir = filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(exe))), "scenarios")
	}

	dbPath := filepath.Join(*dataDir, "benchmarks.db")
	resultsDir := filepath.Join(*dataDir, "results")

	store, err := collector.NewStore(dbPath, resultsDir)
	if err != nil {
		log.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	coll := collector.New(store)
	topoMgr := topology.New(*deployDir)

	if *dbURL != "" {
		topoMgr.SetRemoteURL(*dbURL)
	}

	chaosEngine := chaos.New(topoMgr)
	chaosEngine.SetCallback(func(ev protocol.ChaosEvent) {
		coll.IngestChaosEvent(ev)
	})

	sched := scheduler.New(protocol.DefaultAgents, coll)
	handler := api.NewHandler(topoMgr, sched, chaosEngine, coll, store, *dashDir, *scenariosDir)

	if *topoFlag != "" {
		topo := protocol.Topology(*topoFlag)
		log.Printf("registering topology: %s (assumes CockroachDB is already running via make topo-up)", topo)
		topoMgr.MarkRunning(topo)
	}

	mux := http.NewServeMux()
	handler.Register(mux)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", *port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("controller listening on :%d", *port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("shutting down...")

	chaosEngine.Stop()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}

	_ = topoMgr

	log.Println("controller stopped")
}
