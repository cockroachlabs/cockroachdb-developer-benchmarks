package workloads

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/ddl"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type mixedRWWorkload struct {
	recorder *MetricsRecorder
	rawPool  *pgxpool.Pool
	libPool  *pgxadapter.Pool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	library  bool
}

func RunLibraryMixedRW(rec *MetricsRecorder) WorkloadRunner {
	return &mixedRWWorkload{recorder: rec, library: true}
}

func RunVanillaMixedRW(rec *MetricsRecorder) WorkloadRunner {
	return &mixedRWWorkload{recorder: rec, library: false}
}

func (w *mixedRWWorkload) Start(ctx context.Context, config WorkloadConfig) error {
	ctx, w.cancel = context.WithCancel(ctx)

	if w.library {
		pool, err := pgxadapter.NewPool(ctx, config.DBURL, []crdbgo.Option{
			crdbgo.WithMaxRetries(5),
			crdbgo.WithInitialDelay(50 * time.Millisecond),
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

func (w *mixedRWWorkload) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *mixedRWWorkload) worker(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		var err error

		if rand.Float64() < 0.8 {
			err = w.doRead(ctx)
		} else {
			err = w.doWrite(ctx)
		}

		latencyUs := time.Since(start).Microseconds()
		if err != nil {
			w.recorder.RecordError(err)
		} else {
			w.recorder.RecordOp(latencyUs)
		}
	}
}

func (w *mixedRWWorkload) doRead(ctx context.Context) error {
	accountID := rand.Int63n(100_000) + 1
	rows, err := w.rawPool.Query(ctx, `
		SELECT a.name, a.balance, o.amount, o.status
		FROM bench_accounts a
		LEFT JOIN bench_orders o ON o.account_id = a.id
		WHERE a.id = $1
		LIMIT 20`, accountID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var name, status string
		var balance, amount float64
		if err := rows.Scan(&name, &balance, &amount, &status); err != nil {
			break
		}
	}
	return rows.Err()
}

func (w *mixedRWWorkload) doWrite(ctx context.Context) error {
	if w.library {
		return w.libPool.ExecuteTx(ctx, func(tx pgx.Tx) error {
			accountID := rand.Int63n(100_000) + 1
			amount := rand.Float64() * 500

			_, err := tx.Exec(ctx, `
				INSERT INTO bench_orders (id, account_id, amount, status)
				VALUES ($1, $2, $3, 'pending')`,
				rand.Int63n(100_000_000), accountID, amount)
			if err != nil {
				return err
			}

			_, err = tx.Exec(ctx,
				"UPDATE bench_accounts SET balance = balance - $1 WHERE id = $2",
				amount, accountID)
			return err
		})
	}

	tx, err := w.rawPool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	accountID := rand.Int63n(100_000) + 1
	amount := rand.Float64() * 500

	_, err = tx.Exec(ctx, `
		INSERT INTO bench_orders (id, account_id, amount, status)
		VALUES ($1, $2, $3, 'pending')`,
		rand.Int63n(100_000_000), accountID, amount)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		"UPDATE bench_accounts SET balance = balance - $1 WHERE id = $2",
		amount, accountID)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (w *mixedRWWorkload) ensureSchema(ctx context.Context) error {
	if w.library {
		accountsSQL := ddl.CreateTableWithIndexes("bench_accounts",
			[]ddl.ColumnDef{
				{Name: "id", Type: "INT8", PrimaryKey: true},
				{Name: "name", Type: "STRING", NotNull: true},
				{Name: "balance", Type: "DECIMAL(15,2)", NotNull: true, Default: "0"},
				{Name: "created_at", Type: "TIMESTAMPTZ", NotNull: true, Default: "now()"},
			},
			nil,
		)
		_, err := w.rawPool.Exec(ctx, strings.Replace(accountsSQL, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1))
		if err != nil {
			return err
		}

		ordersSQL := ddl.CreateTableWithIndexes("bench_orders",
			[]ddl.ColumnDef{
				{Name: "id", Type: "INT8", PrimaryKey: true},
				{Name: "account_id", Type: "INT8", NotNull: true},
				{Name: "amount", Type: "DECIMAL(15,2)", NotNull: true},
				{Name: "status", Type: "STRING", NotNull: true, Default: "'pending'"},
				{Name: "created_at", Type: "TIMESTAMPTZ", NotNull: true, Default: "now()"},
			},
			[]ddl.IndexDef{
				{Name: "idx_orders_account", Columns: []string{"account_id"}},
				{Name: "idx_orders_ts", Columns: []string{"created_at DESC"}},
			},
		)
		_, err = w.rawPool.Exec(ctx, strings.Replace(ordersSQL, "CREATE TABLE ", "CREATE TABLE IF NOT EXISTS ", 1))
		return err
	}

	_, err := w.rawPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_accounts (
			id INT8 PRIMARY KEY,
			name STRING NOT NULL,
			balance DECIMAL(15,2) NOT NULL DEFAULT 0,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return err
	}
	_, err = w.rawPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_orders (
			id INT8 PRIMARY KEY,
			account_id INT8 NOT NULL REFERENCES bench_accounts(id),
			amount DECIMAL(15,2) NOT NULL,
			status STRING NOT NULL DEFAULT 'pending',
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func (w *mixedRWWorkload) seedData(ctx context.Context) error {
	var count int
	w.rawPool.QueryRow(ctx, "SELECT count(*) FROM bench_accounts").Scan(&count)
	if count >= 1000 {
		return nil
	}
	for i := 1; i <= 1000; i++ {
		_, err := w.rawPool.Exec(ctx,
			"UPSERT INTO bench_accounts (id, name, balance) VALUES ($1, $2, $3)",
			int64(i), fmt.Sprintf("account-%d", i), 10000.0)
		if err != nil {
			return err
		}
	}
	return nil
}
