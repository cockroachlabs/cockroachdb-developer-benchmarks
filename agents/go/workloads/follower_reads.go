package workloads

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5/pgxpool"
)

type followerReadsWorkload struct {
	recorder *MetricsRecorder
	rawPool  *pgxpool.Pool
	libPool  *pgxadapter.Pool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	library  bool
}

func RunLibraryFollowerReads(rec *MetricsRecorder) WorkloadRunner {
	return &followerReadsWorkload{recorder: rec, library: true}
}

func RunVanillaFollowerReads(rec *MetricsRecorder) WorkloadRunner {
	return &followerReadsWorkload{recorder: rec, library: false}
}

func (w *followerReadsWorkload) Start(ctx context.Context, config WorkloadConfig) error {
	ctx, w.cancel = context.WithCancel(ctx)

	if w.library {
		pool, err := pgxadapter.NewPool(ctx, config.DBURL, []crdbgo.Option{
			crdbgo.WithFollowerReads(5 * time.Second),
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
	if err := w.seedData(ctx); err != nil {
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
		go w.worker(ctx)
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

func (w *followerReadsWorkload) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *followerReadsWorkload) worker(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		var err error

		if rand.Float64() < 0.7 {
			err = w.doFollowerRead(ctx)
		} else {
			err = w.doCurrentRead(ctx)
		}

		latencyUs := time.Since(start).Microseconds()
		if err != nil {
			w.recorder.RecordError(err)
		} else {
			w.recorder.RecordOp(latencyUs)
		}
	}
}

func (w *followerReadsWorkload) doFollowerRead(ctx context.Context) error {
	accountID := rand.Int63n(10_000) + 1

	if w.library {
		var name string
		var balance float64
		return w.libPool.QueryRowFollowerRead(ctx,
			"SELECT name, balance FROM bench_accounts WHERE id = $1",
			accountID).Scan(&name, &balance)
	}

	var name string
	var balance float64
	return w.rawPool.QueryRow(ctx,
		"SELECT name, balance FROM bench_accounts AS OF SYSTEM TIME follower_read_timestamp() WHERE id = $1",
		accountID).Scan(&name, &balance)
}

func (w *followerReadsWorkload) doCurrentRead(ctx context.Context) error {
	accountID := rand.Int63n(10_000) + 1
	var name string
	var balance float64
	return w.rawPool.QueryRow(ctx,
		"SELECT name, balance FROM bench_accounts WHERE id = $1",
		accountID).Scan(&name, &balance)
}

func (w *followerReadsWorkload) ensureSchema(ctx context.Context) error {
	_, err := w.rawPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_accounts (
			id INT8 PRIMARY KEY,
			name STRING NOT NULL,
			balance DECIMAL(15,2) NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func (w *followerReadsWorkload) seedData(ctx context.Context) error {
	var count int
	w.rawPool.QueryRow(ctx, "SELECT count(*) FROM bench_accounts").Scan(&count)
	if count >= 1000 {
		return nil
	}
	for i := 1; i <= 1000; i++ {
		_, err := w.rawPool.Exec(ctx,
			"UPSERT INTO bench_accounts (id, name, balance) VALUES ($1, $2, $3)",
			int64(i), fmt.Sprintf("follower-account-%d", i), 5000.0)
		if err != nil {
			return err
		}
	}
	return nil
}
