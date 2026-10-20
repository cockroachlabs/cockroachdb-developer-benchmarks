package workloads

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/cdc"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5/pgxpool"
)

type cdcWorkload struct {
	recorder  *MetricsRecorder
	rawPool   *pgxpool.Pool
	libPool   *pgxadapter.Pool
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	library   bool
	eventLag  atomic.Int64
	httpSrv   *http.Server
}

func RunLibraryCDC(rec *MetricsRecorder) WorkloadRunner {
	return &cdcWorkload{recorder: rec, library: true}
}

func RunVanillaCDC(rec *MetricsRecorder) WorkloadRunner {
	return &cdcWorkload{recorder: rec, library: false}
}

func (w *cdcWorkload) Start(ctx context.Context, config WorkloadConfig) error {
	ctx, w.cancel = context.WithCancel(ctx)

	if w.library {
		pool, err := pgxadapter.NewPool(ctx, config.DBURL, []crdbgo.Option{
			crdbgo.WithMaxRetries(3),
		})
		if err != nil {
			return err
		}
		w.libPool = pool
		w.rawPool = pool.RawPool()
	} else {
		poolConfig, err := pgxpool.ParseConfig(config.DBURL)
		if err != nil {
			return err
		}
		poolConfig.MaxConns = int32(config.Concurrency + 5)
		w.rawPool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			return err
		}
	}

	if err := w.ensureSchema(ctx); err != nil {
		return err
	}

	var deadline <-chan time.Time
	if config.DurationSecs > 0 {
		timer := time.NewTimer(time.Duration(config.DurationSecs) * time.Second)
		deadline = timer.C
		defer timer.Stop()
	}

	for i := 0; i < config.Concurrency; i++ {
		w.wg.Add(1)
		go w.writeWorker(ctx)
	}

	w.wg.Add(1)
	if w.library {
		go w.libraryChangefeedConsumer(ctx)
	} else {
		go w.changefeedConsumer(ctx)
	}

	if deadline != nil {
		select {
		case <-deadline:
			w.Stop()
		case <-ctx.Done():
		}
	} else {
		<-ctx.Done()
	}

	w.wg.Wait()
	if w.libPool != nil {
		w.libPool.Close()
	} else {
		w.rawPool.Close()
	}
	return nil
}

func (w *cdcWorkload) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	if w.httpSrv != nil {
		w.httpSrv.Close()
	}
}

func (w *cdcWorkload) writeWorker(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		id := rand.Int63n(1_000_000)
		_, err := w.rawPool.Exec(ctx,
			"UPSERT INTO bench_accounts (id, name, balance) VALUES ($1, $2, $3)",
			id, fmt.Sprintf("cdc-account-%d", id), rand.Float64()*10000)

		latencyUs := time.Since(start).Microseconds()
		if err != nil {
			w.recorder.RecordError(err)
		} else {
			w.recorder.RecordOp(latencyUs)
		}
	}
}

func (w *cdcWorkload) libraryChangefeedConsumer(ctx context.Context) {
	defer w.wg.Done()

	handler := cdc.NewWebhookHandler(func(ctx context.Context, events []cdc.Event) error {
		for range events {
			w.recorder.RecordOp(1)
		}
		return nil
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		w.recorder.RecordError(err)
		return
	}
	webhookPort := listener.Addr().(*net.TCPAddr).Port
	webhookURL := fmt.Sprintf("http://127.0.0.1:%d/cdc", webhookPort)

	mux := http.NewServeMux()
	mux.Handle("/cdc", handler)
	w.httpSrv = &http.Server{Handler: mux}
	go w.httpSrv.Serve(listener)

	spec, err := cdc.NewChangefeedSpec("bench_accounts").
		WebhookURL(webhookURL).
		WithFormat("json").
		Updated().
		SkipTLSVerify().
		Build()
	if err != nil {
		w.recorder.RecordError(err)
		return
	}

	_, execErr := w.rawPool.Exec(ctx, spec.ToSQL())
	if execErr != nil {
		w.recorder.RecordError(execErr)
		return
	}

	<-ctx.Done()
}

func (w *cdcWorkload) changefeedConsumer(ctx context.Context) {
	defer w.wg.Done()

	conn, err := w.rawPool.Acquire(ctx)
	if err != nil {
		w.recorder.RecordError(err)
		return
	}
	defer conn.Release()

	query := "EXPERIMENTAL CHANGEFEED FOR bench_accounts WITH updated, resolved = '1s'"

	rows, err := conn.Query(ctx, query)
	if err != nil {
		w.recorder.RecordError(err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var table, key, value []byte
		if err := rows.Scan(&table, &key, &value); err != nil {
			w.recorder.RecordError(err)
			continue
		}

		w.recorder.RecordOp(1)
	}
}

func (w *cdcWorkload) ensureSchema(ctx context.Context) error {
	_, err := w.rawPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_accounts (
			id INT8 PRIMARY KEY,
			name STRING NOT NULL,
			balance DECIMAL(15,2) NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}
