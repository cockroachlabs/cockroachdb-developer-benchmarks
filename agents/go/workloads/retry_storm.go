package workloads

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type retryStormWorkload struct {
	recorder *MetricsRecorder
	rawPool  *pgxpool.Pool
	libPool  *pgxadapter.Pool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	library  bool
}

func RunLibraryRetryStorm(rec *MetricsRecorder) WorkloadRunner {
	return &retryStormWorkload{recorder: rec, library: true}
}

func RunVanillaRetryStorm(rec *MetricsRecorder) WorkloadRunner {
	return &retryStormWorkload{recorder: rec, library: false}
}

func (w *retryStormWorkload) Start(ctx context.Context, config WorkloadConfig) error {
	ctx, w.cancel = context.WithCancel(ctx)

	if w.library {
		pool, err := pgxadapter.NewPool(ctx, config.DBURL, []crdbgo.Option{
			crdbgo.WithMaxRetries(10),
			crdbgo.WithInitialDelay(50 * time.Millisecond),
			crdbgo.WithMaxDelay(2 * time.Second),
			crdbgo.WithMultiplier(2.0),
			crdbgo.WithJitter(true),
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

	hotRows := config.HotRows
	if hotRows == 0 {
		hotRows = 10
	}

	if err := w.seedHotRows(ctx, hotRows); err != nil {
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
		go w.worker(ctx, hotRows)
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

func (w *retryStormWorkload) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *retryStormWorkload) worker(ctx context.Context, hotRows int) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		rowID := rand.Int63n(int64(hotRows)) + 1
		start := time.Now()
		var err error

		if w.library {
			err = w.libraryRetryTxn(ctx, rowID)
		} else {
			err = w.vanillaRetryTxn(ctx, rowID)
		}

		latencyUs := time.Since(start).Microseconds()
		if err != nil {
			w.recorder.RecordError(err)
		} else {
			w.recorder.RecordOp(latencyUs)
		}
	}
}

func (w *retryStormWorkload) libraryRetryTxn(ctx context.Context, rowID int64) error {
	return w.libPool.ExecuteTx(ctx, func(tx pgx.Tx) error {
		var balance float64
		err := tx.QueryRow(ctx,
			"SELECT balance FROM bench_accounts WHERE id = $1 FOR UPDATE", rowID).Scan(&balance)
		if err != nil {
			return err
		}

		delta := rand.Float64()*200 - 100
		_, err = tx.Exec(ctx,
			"UPDATE bench_accounts SET balance = $1 WHERE id = $2", balance+delta, rowID)
		return err
	})
}

func (w *retryStormWorkload) vanillaRetryTxn(ctx context.Context, rowID int64) error {
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := w.execTxn(ctx, rowID)
		if err == nil {
			if attempt > 0 {
				w.recorder.RecordRetrySuccess()
			}
			return nil
		}
		if !isRetryable(err) {
			return err
		}
		w.recorder.RecordRetry()
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return fmt.Errorf("max retries exceeded for row %d", rowID)
}

func (w *retryStormWorkload) execTxn(ctx context.Context, rowID int64) error {
	tx, err := w.rawPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var balance float64
	err = tx.QueryRow(ctx,
		"SELECT balance FROM bench_accounts WHERE id = $1 FOR UPDATE", rowID).Scan(&balance)
	if err != nil {
		return err
	}

	delta := rand.Float64()*200 - 100
	_, err = tx.Exec(ctx,
		"UPDATE bench_accounts SET balance = $1 WHERE id = $2", balance+delta, rowID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001"
	}
	return false
}

func (w *retryStormWorkload) ensureSchema(ctx context.Context) error {
	_, err := w.rawPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_accounts (
			id INT8 PRIMARY KEY,
			name STRING NOT NULL,
			balance DECIMAL(15,2) NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func (w *retryStormWorkload) seedHotRows(ctx context.Context, count int) error {
	for i := 1; i <= count; i++ {
		_, err := w.rawPool.Exec(ctx,
			"UPSERT INTO bench_accounts (id, name, balance) VALUES ($1, $2, $3)",
			int64(i), fmt.Sprintf("hot-account-%d", i), 1000.0)
		if err != nil {
			return err
		}
	}
	return nil
}
