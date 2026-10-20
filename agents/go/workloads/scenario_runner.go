package workloads

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ScenarioOperation struct {
	Name        string         `json:"name"`
	Type        string         `json:"type"`
	Concurrency int            `json:"concurrency"`
	Weight      int            `json:"weight"`
	SQL         []string       `json:"sql"`
	Params      map[string]any `json:"params,omitempty"`
	LibraryMode *struct {
		UseRetry        bool `json:"use_retry"`
		UseTemplate     bool `json:"use_template"`
		UseFollowerRead bool `json:"use_follower_read"`
	} `json:"library_mode,omitempty"`
}

type ScenarioSchema struct {
	SetupSQL    []string `json:"setup_sql,omitempty"`
	SeedSQL     []string `json:"seed_sql,omitempty"`
	TeardownSQL []string `json:"teardown_sql,omitempty"`
}

type ScenarioDefinition struct {
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	Schema       ScenarioSchema      `json:"schema,omitempty"`
	DurationSecs int                 `json:"duration_secs"`
	WarmupSecs   int                 `json:"warmup_secs,omitempty"`
	Operations   []ScenarioOperation `json:"operations"`
}

type scenarioRunner struct {
	recorder *MetricsRecorder
	pool     *pgxpool.Pool
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	mode     string
	scenario ScenarioDefinition
}

func NewScenarioRunner(rec *MetricsRecorder, scenarioData json.RawMessage, mode string) (WorkloadRunner, error) {
	var sd ScenarioDefinition
	if err := json.Unmarshal(scenarioData, &sd); err != nil {
		return nil, fmt.Errorf("parse scenario data: %w", err)
	}
	if len(sd.Operations) == 0 {
		return nil, fmt.Errorf("scenario has no operations")
	}
	return &scenarioRunner{recorder: rec, mode: mode, scenario: sd}, nil
}

func (s *scenarioRunner) Start(ctx context.Context, config WorkloadConfig) error {
	ctx, s.cancel = context.WithCancel(ctx)

	poolConfig, err := pgxpool.ParseConfig(config.DBURL)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	totalConcurrency := 0
	for _, op := range s.scenario.Operations {
		c := op.Concurrency
		if c <= 0 {
			c = config.Concurrency
		}
		totalConcurrency += c
	}
	poolConfig.MaxConns = int32(totalConcurrency + 5)
	s.pool, err = pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}

	if err := s.runSetup(ctx); err != nil {
		return fmt.Errorf("schema setup: %w", err)
	}

	duration := s.scenario.DurationSecs
	if config.DurationSecs > 0 {
		duration = config.DurationSecs
	}

	var deadline <-chan time.Time
	if duration > 0 {
		timer := time.NewTimer(time.Duration(duration) * time.Second)
		deadline = timer.C
		defer timer.Stop()
	}

	totalWeight := 0
	for _, op := range s.scenario.Operations {
		totalWeight += op.Weight
	}

	for i := range s.scenario.Operations {
		op := s.scenario.Operations[i]
		concurrency := op.Concurrency
		if concurrency <= 0 {
			concurrency = config.Concurrency
		}
		for w := 0; w < concurrency; w++ {
			s.wg.Add(1)
			go s.worker(ctx, op, totalWeight)
		}
	}

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			if s.pool != nil {
				stat := s.pool.Stat()
				s.recorder.SetActiveConns(stat.AcquiredConns())
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()

	if deadline != nil {
		select {
		case <-deadline:
			s.Stop()
		case <-ctx.Done():
		}
	} else {
		<-ctx.Done()
	}

	s.wg.Wait()
	s.pool.Close()
	return nil
}

func (s *scenarioRunner) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *scenarioRunner) runSetup(ctx context.Context) error {
	for _, sql := range s.scenario.Schema.SetupSQL {
		if _, err := s.pool.Exec(ctx, sql); err != nil {
			return fmt.Errorf("setup SQL %q: %w", truncate(sql, 60), err)
		}
	}

	hotRows := s.getGlobalHotRows()
	if hotRows > 0 && len(s.scenario.Schema.SeedSQL) > 0 {
		for _, seedSQL := range s.scenario.Schema.SeedSQL {
			for i := 1; i <= hotRows; i++ {
				_, err := s.pool.Exec(ctx, seedSQL, int64(i), fmt.Sprintf("seed-%d", i))
				if err != nil {
					return fmt.Errorf("seed SQL: %w", err)
				}
			}
		}
	} else if len(s.scenario.Schema.SeedSQL) > 0 {
		seedCount := 1000
		for _, seedSQL := range s.scenario.Schema.SeedSQL {
			for i := 1; i <= seedCount; i++ {
				_, err := s.pool.Exec(ctx, seedSQL, int64(i), fmt.Sprintf("seed-%d", i))
				if err != nil {
					return fmt.Errorf("seed SQL: %w", err)
				}
			}
		}
	}
	return nil
}

func (s *scenarioRunner) worker(ctx context.Context, op ScenarioOperation, _ int) {
	defer s.wg.Done()

	thinkTimeMs := s.getParamFloat(op.Params, "think_time_ms", 0)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		var err error

		switch op.Type {
		case "query":
			err = s.execQuery(ctx, op)
		case "insert":
			err = s.execInsert(ctx, op)
		case "update":
			err = s.execUpdate(ctx, op)
		case "delete":
			err = s.execDelete(ctx, op)
		case "transaction":
			err = s.execTransaction(ctx, op)
		case "follower_read":
			err = s.execFollowerRead(ctx, op)
		default:
			err = fmt.Errorf("unknown operation type: %s", op.Type)
		}

		latencyUs := time.Since(start).Microseconds()
		if err != nil {
			s.recorder.RecordError(err)
		} else {
			s.recorder.RecordOp(latencyUs)
		}

		if thinkTimeMs > 0 {
			time.Sleep(time.Duration(thinkTimeMs) * time.Millisecond)
		}
	}
}

func (s *scenarioRunner) execQuery(ctx context.Context, op ScenarioOperation) error {
	if len(op.SQL) == 0 {
		return nil
	}
	sql := op.SQL[0]
	params := s.buildParams(op)
	rows, err := s.pool.Query(ctx, sql, params...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return err
		}
		_ = values
	}
	return rows.Err()
}

func (s *scenarioRunner) execInsert(ctx context.Context, op ScenarioOperation) error {
	if len(op.SQL) == 0 {
		return nil
	}
	sql := op.SQL[0]
	params := s.buildParams(op)
	_, err := s.pool.Exec(ctx, sql, params...)
	return err
}

func (s *scenarioRunner) execUpdate(ctx context.Context, op ScenarioOperation) error {
	if len(op.SQL) == 0 {
		return nil
	}
	sql := op.SQL[0]
	params := s.buildParams(op)
	_, err := s.pool.Exec(ctx, sql, params...)
	return err
}

func (s *scenarioRunner) execDelete(ctx context.Context, op ScenarioOperation) error {
	if len(op.SQL) == 0 {
		return nil
	}
	sql := op.SQL[0]
	params := s.buildParams(op)
	_, err := s.pool.Exec(ctx, sql, params...)
	return err
}

func (s *scenarioRunner) execTransaction(ctx context.Context, op ScenarioOperation) error {
	maxRetries := 5
	for attempt := 0; attempt < maxRetries; attempt++ {
		err := s.runTxn(ctx, op)
		if err == nil {
			if attempt > 0 {
				s.recorder.RecordRetrySuccess()
			}
			return nil
		}
		if !s.isRetryable(err) {
			return err
		}
		s.recorder.RecordRetry()
		time.Sleep(time.Duration(attempt+1) * 10 * time.Millisecond)
	}
	return fmt.Errorf("max retries exceeded for transaction %s", op.Name)
}

func (s *scenarioRunner) runTxn(ctx context.Context, op ScenarioOperation) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	hotRows := s.getParamFloat(op.Params, "hot_rows", 0)
	amountRange := s.getAmountRange(op.Params)

	for _, sql := range op.SQL {
		params := s.buildTxnParams(sql, hotRows, amountRange)
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(sql)), "SELECT") {
			rows, err := tx.Query(ctx, sql, params...)
			if err != nil {
				return err
			}
			for rows.Next() {
				values, err := rows.Values()
				if err != nil {
					rows.Close()
					return err
				}
				_ = values
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		} else {
			_, err := tx.Exec(ctx, sql, params...)
			if err != nil {
				return err
			}
		}
	}

	return tx.Commit(ctx)
}

func (s *scenarioRunner) execFollowerRead(ctx context.Context, op ScenarioOperation) error {
	if len(op.SQL) == 0 {
		return nil
	}
	sql := op.SQL[0] + " AS OF SYSTEM TIME follower_read_timestamp()"
	params := s.buildParams(op)
	rows, err := s.pool.Query(ctx, sql, params...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return err
		}
		_ = values
	}
	return rows.Err()
}

func (s *scenarioRunner) buildParams(op ScenarioOperation) []any {
	hotRows := s.getParamFloat(op.Params, "hot_rows", 0)
	amountRange := s.getAmountRange(op.Params)

	sql := ""
	if len(op.SQL) > 0 {
		sql = op.SQL[0]
	}

	paramCount := countParams(sql)
	params := make([]any, 0, paramCount)

	for i := 0; i < paramCount; i++ {
		switch {
		case i == 0 && hotRows > 0:
			params = append(params, rand.Int63n(int64(hotRows))+1)
		case i == 0 && (op.Type == "insert" || op.Type == "delete"):
			params = append(params, uuid.New().String())
		case i == 0:
			params = append(params, rand.Int63n(100_000)+1)
		case i == 1 && op.Type == "insert":
			params = append(params, "acct-"+RandomString(8))
		case amountRange != nil:
			low, high := amountRange[0], amountRange[1]
			params = append(params, low+rand.Float64()*(high-low))
		default:
			params = append(params, pickRegionForScenario())
		}
	}

	return params
}

func (s *scenarioRunner) buildTxnParams(sql string, hotRows float64, amountRange []float64) []any {
	paramCount := countParams(sql)
	params := make([]any, 0, paramCount)

	for i := 0; i < paramCount; i++ {
		switch {
		case i == 0 && hotRows > 0:
			params = append(params, rand.Int63n(int64(hotRows))+1)
		case i == 0 && strings.Contains(strings.ToUpper(sql), "INSERT"):
			params = append(params, rand.Int63n(100_000_000))
		case i == 0:
			params = append(params, rand.Int63n(100_000)+1)
		case amountRange != nil:
			low, high := amountRange[0], amountRange[1]
			params = append(params, low+rand.Float64()*(high-low))
		default:
			params = append(params, rand.Int63n(100_000)+1)
		}
	}
	return params
}

func (s *scenarioRunner) getAmountRange(params map[string]any) []float64 {
	if params == nil {
		return nil
	}
	raw, ok := params["amount_range"]
	if !ok {
		return nil
	}
	arr, ok := raw.([]any)
	if !ok || len(arr) != 2 {
		return nil
	}
	low, ok1 := toFloat64(arr[0])
	high, ok2 := toFloat64(arr[1])
	if !ok1 || !ok2 {
		return nil
	}
	return []float64{low, high}
}

func (s *scenarioRunner) getParamFloat(params map[string]any, key string, def float64) float64 {
	if params == nil {
		return def
	}
	raw, ok := params[key]
	if !ok {
		return def
	}
	v, ok := toFloat64(raw)
	if !ok {
		return def
	}
	return v
}

func (s *scenarioRunner) getGlobalHotRows() int {
	for _, op := range s.scenario.Operations {
		if v := s.getParamFloat(op.Params, "hot_rows", 0); v > 0 {
			return int(v)
		}
	}
	return 0
}

func (s *scenarioRunner) isRetryable(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001"
	}
	return false
}

func toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func countParams(sql string) int {
	max := 0
	for i := 0; i < len(sql)-1; i++ {
		if sql[i] == '$' {
			j := i + 1
			n := 0
			for j < len(sql) && sql[j] >= '0' && sql[j] <= '9' {
				n = n*10 + int(sql[j]-'0')
				j++
			}
			if n > max {
				max = n
			}
		}
	}
	return max
}

func pickRegionForScenario() string {
	regions := []string{"us-east", "us-west", "eu-west"}
	return regions[rand.Intn(len(regions))]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func randomVector1536() string {
	parts := make([]string, 1536)
	var norm float64
	vals := make([]float64, 1536)
	for i := range vals {
		v := rand.Float64()*2 - 1
		vals[i] = v
		norm += v * v
	}
	norm = math.Sqrt(norm)
	for i, v := range vals {
		parts[i] = fmt.Sprintf("%g", v/norm)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
