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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type crudWorkload struct {
	recorder *MetricsRecorder
	rawPool  *pgxpool.Pool
	libPool  *pgxadapter.Pool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	library  bool
}

func RunLibraryCRUD(rec *MetricsRecorder) WorkloadRunner {
	return &crudWorkload{recorder: rec, library: true}
}

func RunVanillaCRUD(rec *MetricsRecorder) WorkloadRunner {
	return &crudWorkload{recorder: rec, library: false}
}

func (w *crudWorkload) Start(ctx context.Context, config WorkloadConfig) error {
	ctx, w.cancel = context.WithCancel(ctx)

	if w.library {
		pool, err := pgxadapter.NewPool(ctx, config.DBURL, []crdbgo.Option{
			crdbgo.WithMaxRetries(5),
			crdbgo.WithInitialDelay(50 * time.Millisecond),
			crdbgo.WithJitter(true),
		})
		if err != nil {
			return fmt.Errorf("create pool: %w", err)
		}
		w.libPool = pool
		w.rawPool = pool.RawPool()
	} else {
		poolConfig, err := pgxpool.ParseConfig(config.DBURL)
		if err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
		poolConfig.MaxConns = int32(config.Concurrency + 5)
		w.rawPool, err = pgxpool.NewWithConfig(ctx, poolConfig)
		if err != nil {
			return fmt.Errorf("create pool: %w", err)
		}
	}

	if err := w.ensureSchema(ctx); err != nil {
		return fmt.Errorf("schema: %w", err)
	}

	var deadline <-chan time.Time
	if config.DurationSecs > 0 {
		timer := time.NewTimer(time.Duration(config.DurationSecs) * time.Second)
		deadline = timer.C
		defer timer.Stop()
	}

	for i := 0; i < config.Concurrency; i++ {
		w.wg.Add(1)
		go w.worker(ctx, i)
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if w.rawPool != nil {
				stat := w.rawPool.Stat()
				w.recorder.SetActiveConns(stat.AcquiredConns())
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

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

func (w *crudWorkload) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *crudWorkload) worker(ctx context.Context, id int) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		op := rand.Intn(4)
		start := time.Now()
		var err error

		switch op {
		case 0:
			err = w.doInsert(ctx)
		case 1:
			err = w.doSelect(ctx)
		case 2:
			err = w.doUpdate(ctx)
		case 3:
			err = w.doDelete(ctx)
		}

		latencyUs := time.Since(start).Microseconds()
		if err != nil {
			w.recorder.RecordError(err)
		} else {
			w.recorder.RecordOp(latencyUs)
		}
	}
}

func pickRegion() string {
	switch rand.Intn(3) {
	case 0:
		return "us-east"
	case 1:
		return "us-west"
	default:
		return "eu-west"
	}
}

func (w *crudWorkload) doInsert(ctx context.Context) error {
	id := uuid.New().String()
	name := "acct-" + RandomString(8)
	balance := 1000 + rand.Float64()*9000
	region := pickRegion()
	if w.library {
		return w.libPool.ExecuteTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				"INSERT INTO bench_accounts (id, name, balance, region) VALUES ($1, $2, $3, $4)",
				id, name, balance, region)
			return err
		})
	}
	_, err := w.rawPool.Exec(ctx,
		"INSERT INTO bench_accounts (id, name, balance, region) VALUES ($1, $2, $3, $4)",
		id, name, balance, region)
	return err
}

func (w *crudWorkload) doSelect(ctx context.Context) error {
	var name string
	var balance float64
	err := w.rawPool.QueryRow(ctx,
		"SELECT name, balance FROM bench_accounts WHERE id = $1",
		uuid.New().String()).Scan(&name, &balance)
	// Selecting a random (non-existent) UUID legitimately returns no rows;
	// that is a successful probe, not an error (matches the other agents).
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return err
}

func (w *crudWorkload) doUpdate(ctx context.Context) error {
	id := uuid.New().String()
	if w.library {
		return w.libPool.ExecuteTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				"UPDATE bench_accounts SET balance = balance + $1, updated_at = now() WHERE id = $2",
				rand.Float64()*100-50, id)
			return err
		})
	}
	_, err := w.rawPool.Exec(ctx,
		"UPDATE bench_accounts SET balance = balance + $1, updated_at = now() WHERE id = $2",
		rand.Float64()*100-50, id)
	return err
}

func (w *crudWorkload) doDelete(ctx context.Context) error {
	id := uuid.New().String()
	if w.library {
		return w.libPool.ExecuteTx(ctx, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx,
				"DELETE FROM bench_accounts WHERE id = $1", id)
			return err
		})
	}
	_, err := w.rawPool.Exec(ctx,
		"DELETE FROM bench_accounts WHERE id = $1", id)
	return err
}

func (w *crudWorkload) ensureSchema(ctx context.Context) error {
	_, err := w.rawPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_accounts (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			balance DECIMAL(15,2) NOT NULL DEFAULT 1000.00,
			name STRING NOT NULL,
			region STRING NOT NULL DEFAULT 'us-east',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
			updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}
