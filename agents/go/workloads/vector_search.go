package workloads

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/ai"
	"github.com/felipeg48/cockroachdb-go-library/crdbgo/pgxadapter"
	"github.com/jackc/pgx/v5/pgxpool"
)

const vectorDim = 1536

type vectorSearchWorkload struct {
	recorder *MetricsRecorder
	rawPool  *pgxpool.Pool
	libPool  *pgxadapter.Pool
	template *ai.VectorSearchTemplate
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	library  bool
	sqlDB    *sql.DB
}

func RunLibraryVectorSearch(rec *MetricsRecorder) WorkloadRunner {
	return &vectorSearchWorkload{recorder: rec, library: true}
}

func RunVanillaVectorSearch(rec *MetricsRecorder) WorkloadRunner {
	return &vectorSearchWorkload{recorder: rec, library: false}
}

func (w *vectorSearchWorkload) Start(ctx context.Context, config WorkloadConfig) error {
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

		w.sqlDB, err = sql.Open("pgx", config.DBURL)
		if err != nil {
			return err
		}
		w.template = ai.NewVectorSearchTemplate(w.sqlDB)
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

	if err := w.seedDocuments(ctx, 500); err != nil {
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
	if w.sqlDB != nil {
		w.sqlDB.Close()
	}
	return nil
}

func (w *vectorSearchWorkload) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}

func (w *vectorSearchWorkload) worker(ctx context.Context) {
	defer w.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		var err error

		if rand.Float64() < 0.6 {
			err = w.doSearch(ctx)
		} else {
			err = w.doInsert(ctx)
		}

		latencyUs := time.Since(start).Microseconds()
		if err != nil {
			w.recorder.RecordError(err)
		} else {
			w.recorder.RecordOp(latencyUs)
		}
	}
}

func (w *vectorSearchWorkload) doInsert(ctx context.Context) error {
	vec := randomVector(vectorDim)

	if w.library {
		return w.template.StoreEmbedding(ctx, ai.StoreOptions{
			Table:        "bench_documents",
			IDColumn:     "id",
			VectorColumn: "embedding",
			ID:           fmt.Sprintf("%d", rand.Int63n(10_000_000)),
			Embedding:    vec,
			Metadata: map[string]any{
				"title":   fmt.Sprintf("doc-%s", RandomString(8)),
				"content": RandomString(200),
			},
		})
	}

	vecStr := vectorToString(vec)
	_, err := w.rawPool.Exec(ctx,
		"INSERT INTO bench_documents (id, title, content, embedding) VALUES ($1, $2, $3, $4::FLOAT8[])",
		rand.Int63n(10_000_000),
		fmt.Sprintf("doc-%s", RandomString(8)),
		RandomString(200),
		vecStr)
	return err
}

func (w *vectorSearchWorkload) doSearch(ctx context.Context) error {
	queryVec := randomVector(vectorDim)

	if w.library {
		w.rawPool.Exec(ctx, "SET vector_search_beam_size = 32")
		results, err := w.template.Search(ctx, ai.SearchOptions{
			Table:        "bench_documents",
			VectorColumn: "embedding",
			QueryVector:  queryVec,
			Metric:       ai.Cosine,
			Limit:        10,
			Columns:      []string{"id", "title"},
		})
		if err != nil {
			return err
		}
		_ = results
		return nil
	}

	vecStr := vectorToString(queryVec)
	query := `
		SELECT id, title,
			(SELECT sum(a.v * b.v)
			 FROM unnest(embedding) WITH ORDINALITY AS a(v, i),
			      unnest($1::FLOAT8[]) WITH ORDINALITY AS b(v, i)
			 WHERE a.i = b.i) AS similarity
		FROM bench_documents
		ORDER BY similarity DESC
		LIMIT 10`

	rows, err := w.rawPool.Query(ctx, query, vecStr)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id int64
		var title string
		var similarity float64
		if err := rows.Scan(&id, &title, &similarity); err != nil {
			return err
		}
	}
	return rows.Err()
}

func (w *vectorSearchWorkload) ensureSchema(ctx context.Context) error {
	if w.library {
		return w.template.InitSchemaV26(ctx, "bench_documents", "embedding", vectorDim, &ai.VectorIndexOptions{
			Metric: ai.Cosine,
		})
	}
	_, err := w.rawPool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS bench_documents (
			id INT8 PRIMARY KEY,
			title STRING NOT NULL,
			content STRING NOT NULL,
			embedding FLOAT8[] NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	return err
}

func (w *vectorSearchWorkload) seedDocuments(ctx context.Context, count int) error {
	var existing int
	w.rawPool.QueryRow(ctx, "SELECT count(*) FROM bench_documents").Scan(&existing)
	if existing >= count {
		return nil
	}

	for i := 0; i < count; i++ {
		vec := randomVector(vectorDim)
		vecStr := vectorToString(vec)
		_, err := w.rawPool.Exec(ctx,
			"UPSERT INTO bench_documents (id, title, content, embedding) VALUES ($1, $2, $3, $4::FLOAT8[])",
			int64(i+1),
			fmt.Sprintf("seed-doc-%d", i+1),
			RandomString(200),
			vecStr)
		if err != nil {
			return err
		}
	}
	return nil
}

func randomVector(dim int) []float32 {
	vec := make([]float32, dim)
	var norm float64
	for i := range vec {
		v := rand.Float32()*2 - 1
		vec[i] = v
		norm += float64(v * v)
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] = float32(float64(vec[i]) / norm)
	}
	return vec
}

func vectorToString(vec []float32) string {
	parts := make([]string, len(vec))
	for i, v := range vec {
		parts[i] = fmt.Sprintf("%g", v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}
