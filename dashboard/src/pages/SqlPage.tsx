import { useState, useEffect, useRef } from 'react'
import { type WorkloadType, WORKLOAD_TYPES, WORKLOAD_LABELS, WORKLOAD_INFO, LANGUAGE_COLORS } from '../types'
import Prism from 'prismjs'
import 'prismjs/components/prism-java'
import 'prismjs/components/prism-go'
import 'prismjs/components/prism-python'
import 'prismjs/components/prism-csharp'
import 'prismjs/components/prism-sql'

const PRISM_LANG: Record<string, string> = {
  java: 'java',
  go: 'go',
  python: 'python',
  dotnet: 'csharp',
};

function CodeBlock({ code, lang }: { code: string; lang: string }) {
  const ref = useRef<HTMLElement>(null);
  const prismLang = PRISM_LANG[lang] || 'clike';

  useEffect(() => {
    if (ref.current) {
      ref.current.textContent = code;
      Prism.highlightElement(ref.current);
    }
  }, [code, prismLang]);

  return (
    <pre style={{ margin: 0, background: 'transparent', padding: 0, overflow: 'auto' }}>
      <code ref={ref} className={`language-${prismLang}`} style={{ fontSize: 11, lineHeight: 1.5 }}>
        {code}
      </code>
    </pre>
  );
}

type Lang = 'java' | 'go' | 'python' | 'dotnet';
const LANGS: { key: Lang; label: string }[] = [
  { key: 'java', label: 'Java' },
  { key: 'go', label: 'Go' },
  { key: 'python', label: 'Python' },
  { key: 'dotnet', label: '.NET' },
];

interface SqlEntry {
  operation: string;
  library: Record<Lang, string>;
  vanilla: Record<Lang, string>;
  note?: string;
}

const WORKLOAD_SQL: Record<WorkloadType, SqlEntry[]> = {
  crud: [
    {
      operation: 'INSERT',
      library: {
        java: `// Using @CockroachDB annotation\n@CockroachDB(maxRetries = 5)\npublic void createAccount(String id,\n    String name, BigDecimal balance,\n    String region) {\n  accountRepo.save(\n    new Account(id, name, balance, region));\n}\n\n// Or programmatic with CockroachDBTemplate\ncrdbTemplate.execute(() -> {\n  accountRepo.save(account);\n  return null;\n});`,
        go: `// pgxadapter.Pool.ExecuteTx\npool.ExecuteTx(ctx, func(tx pgx.Tx) error {\n  _, err := tx.Exec(ctx,\n    "INSERT INTO bench_accounts\n      (id, name, balance, region)\n      VALUES ($1, $2, $3, $4)",\n    uuid, name, balance, region)\n  return err\n})`,
        python: `# CockroachDBSession with cockroachdb:// dialect\ncrdb = CockroachDBSession(engine)\ncrdb.execute_with_retry(lambda s:\n  s.execute(text(\n    "INSERT INTO bench_accounts\n      (id, name, balance, region)\n      VALUES (:id, :name, :bal, :reg)"),\n    {"id": uid, "name": name,\n     "bal": balance, "reg": region}))`,
        dotnet: `// ICockroachDBTemplate pipeline\nawait template.ExecuteAsync(async (conn) => {\n  await conn.ExecuteNonQueryAsync(\n    "INSERT INTO bench_accounts\n      (id, name, balance, region)\n      VALUES ($1, $2, $3, $4)",\n    uuid, name, balance, region);\n});`,
      },
      vanilla: {
        java: `// Raw JDBC\ntry (Connection conn = ds.getConnection();\n     PreparedStatement ps = conn.prepareStatement(\n       "INSERT INTO bench_accounts\n         (id, name, balance, region)\n         VALUES (?, ?, ?, ?)")) {\n  ps.setObject(1, uuid);\n  ps.setString(2, name);\n  ps.setBigDecimal(3, balance);\n  ps.setString(4, region);\n  ps.executeUpdate();\n}`,
        go: `// Raw pgx\npool.Exec(ctx,\n  "INSERT INTO bench_accounts\n    (id, name, balance, region)\n    VALUES ($1, $2, $3, $4)",\n  uuid, name, balance, region)`,
        python: `# Raw psycopg3\nwith pool.connection() as conn:\n  conn.execute(\n    "INSERT INTO bench_accounts\n      (id, name, balance, region)\n      VALUES (%s, %s, %s, %s)",\n    (uuid, name, balance, region))`,
        dotnet: `// Raw Npgsql\nawait using var cmd = dataSource\n  .CreateCommand(\n    "INSERT INTO bench_accounts\n      (id, name, balance, region)\n      VALUES ($1, $2, $3, $4)");\ncmd.Parameters.AddWithValue(uuid);\ncmd.Parameters.AddWithValue(name);\ncmd.Parameters.AddWithValue(balance);\ncmd.Parameters.AddWithValue(region);\nawait cmd.ExecuteNonQueryAsync();`,
      },
      note: 'Library wraps in retry-capable transaction',
    },
    {
      operation: 'UPDATE with Retry',
      library: {
        java: `// @CockroachDB declarative retry\n@CockroachDB(maxRetries = 10, delay = 50,\n  multiplier = 2.0)\npublic void updateBalance(String id,\n    BigDecimal amount) {\n  accountRepo.addBalance(id, amount);\n}\n\n// Programmatic\ncrdbTemplate.execute(() -> {\n  accountRepo.addBalance(id, amount);\n  return null;\n});\n// Handles SQLSTATE 40001 internally\n// Exponential backoff + jitter`,
        go: `// pgxadapter handles 40001 internally\npool.ExecuteTx(ctx, func(tx pgx.Tx) error {\n  _, err := tx.Exec(ctx,\n    "UPDATE bench_accounts\n      SET balance = balance + $1\n      WHERE id = $2",\n    amount, id)\n  return err\n})\n// Config: WithMaxRetries(5), WithJitter(true)`,
        python: `# Savepoint restart protocol via dialect\ncrdb.execute_with_retry(lambda s:\n  s.execute(text(\n    "UPDATE bench_accounts\n      SET balance = balance + :amt\n      WHERE id = :id"),\n    {"amt": amount, "id": uid}))\n# Uses SAVEPOINT cockroach_restart`,
        dotnet: `// Pipeline: RetryMiddleware + CircuitBreaker\nawait template.ExecuteAsync(async (conn) => {\n  await conn.ExecuteNonQueryAsync(\n    "UPDATE bench_accounts\n      SET balance = balance + $1\n      WHERE id = $2",\n    amount, id);\n});\n// Polly v8: backoff + circuit breaker`,
      },
      vanilla: {
        java: `// Manual retry loop\nfor (int i = 0; i < 5; i++) {\n  try {\n    jdbcTemplate.update(\n      "UPDATE bench_accounts\n        SET balance = balance + ?\n        WHERE id = ?",\n      amount, id);\n    break;\n  } catch (SQLException e) {\n    if (!"40001".equals(e.getSQLState()))\n      throw e;\n    Thread.sleep(100); // fixed delay\n  }\n}`,
        go: `// Manual retry\nfor attempt := 0; attempt < 5; attempt++ {\n  _, err := pool.Exec(ctx,\n    "UPDATE bench_accounts\n      SET balance = balance + $1\n      WHERE id = $2",\n    amount, id)\n  if err == nil { break }\n  var pgErr *pgconn.PgError\n  if !errors.As(err, &pgErr) ||\n     pgErr.Code != "40001" { break }\n  time.Sleep(100 * time.Millisecond)\n}`,
        python: `# Manual retry\nfor attempt in range(5):\n  try:\n    with pool.connection() as conn:\n      conn.execute(\n        "UPDATE bench_accounts\n          SET balance = balance + %s\n          WHERE id = %s",\n        (amount, uid))\n    break\n  except SerializationFailure:\n    time.sleep(0.1)`,
        dotnet: `// Manual retry with Npgsql\nfor (var i = 0; i < 5; i++) {\n  try {\n    await cmd.ExecuteNonQueryAsync();\n    break;\n  } catch (NpgsqlException ex)\n    when (ex.SqlState == "40001") {\n    await Task.Delay(100);\n  }\n}`,
      },
      note: 'Key difference: library handles SQLSTATE 40001 automatically',
    },
  ],
  retry_storm: [
    {
      operation: 'Hot Row Transfer (32 workers, 10 rows)',
      library: {
        java: `// @CockroachDB handles hot-row contention\n@CockroachDB(maxRetries = 10, delay = 50,\n  multiplier = 2.0)\npublic void transfer(String src,\n    String dst, BigDecimal amt) {\n  accountRepo.debit(src, amt);\n  accountRepo.credit(dst, amt);\n}\n\n// Programmatic\ncrdbTemplate.execute(() -> {\n  accountRepo.debit(src, amt);\n  accountRepo.credit(dst, amt);\n  return null;\n});\n// 10 retries, 50ms base, 2x multiplier,\n// jitter prevents thundering herd`,
        go: `// crdbgo retry config\npool, _ := pgxadapter.NewPool(ctx, url,\n  []crdbgo.Option{\n    crdbgo.WithMaxRetries(10),\n    crdbgo.WithInitialDelay(50*time.Millisecond),\n    crdbgo.WithMultiplier(2.0),\n    crdbgo.WithJitter(true),\n  })\npool.ExecuteTx(ctx, func(tx pgx.Tx) error {\n  tx.Exec(ctx, "UPDATE ... - $1 WHERE id = $2", amt, src)\n  tx.Exec(ctx, "UPDATE ... + $1 WHERE id = $2", amt, dst)\n  return nil\n})`,
        python: `# Savepoint restart — server-side retry\nengine = create_engine("cockroachdb://...")\ncrdb = CockroachDBSession(engine)\ncrdb.execute_with_retry(lambda s:\n  s.execute(text("UPDATE ... - :a WHERE id = :s"),\n    {"a": amt, "s": src});\n  s.execute(text("UPDATE ... + :a WHERE id = :d"),\n    {"a": amt, "d": dst}))\n# CockroachDB restarts txn server-side`,
        dotnet: `// Pipeline: Retry + CircuitBreaker\nawait template.ExecuteAsync(async (conn) => {\n  await conn.ExecuteNonQueryAsync(\n    "UPDATE ... - $1 WHERE id = $2", amt, src);\n  await conn.ExecuteNonQueryAsync(\n    "UPDATE ... + $1 WHERE id = $2", amt, dst);\n});\n// Polly: 10 retries, exp backoff, jitter`,
      },
      vanilla: {
        java: `// Manual BEGIN/COMMIT + 40001 check\nfor (int i = 0; i < 5; i++) {\n  Connection conn = ds.getConnection();\n  conn.setAutoCommit(false);\n  try {\n    stmt.executeUpdate("UPDATE ... - ? WHERE id = ?");\n    stmt.executeUpdate("UPDATE ... + ? WHERE id = ?");\n    conn.commit();\n    break;\n  } catch (SQLException e) {\n    conn.rollback();\n    if (!"40001".equals(e.getSQLState())) throw e;\n    Thread.sleep(100); // no jitter\n  }\n}`,
        go: `// Manual retry with fixed delay\nfor attempt := 0; attempt < 5; attempt++ {\n  tx, _ := pool.Begin(ctx)\n  tx.Exec(ctx, "UPDATE ... - $1 WHERE id = $2", amt, src)\n  tx.Exec(ctx, "UPDATE ... + $1 WHERE id = $2", amt, dst)\n  err := tx.Commit(ctx)\n  if err == nil { break }\n  // No jitter → thundering herd\n  time.Sleep(100 * time.Millisecond)\n}`,
        python: `# Manual retry — no savepoint restart\nfor attempt in range(5):\n  try:\n    with pool.connection() as conn:\n      with conn.transaction():\n        conn.execute("UPDATE ... - %s WHERE id = %s")\n        conn.execute("UPDATE ... + %s WHERE id = %s")\n    break\n  except SerializationFailure:\n    time.sleep(0.1) # fixed, no jitter`,
        dotnet: `// Manual retry — linear backoff\nfor (var i = 0; i < 5; i++) {\n  await using var tx = await conn\n    .BeginTransactionAsync();\n  try {\n    await cmd1.ExecuteNonQueryAsync();\n    await cmd2.ExecuteNonQueryAsync();\n    await tx.CommitAsync();\n    break;\n  } catch (NpgsqlException ex)\n    when (ex.SqlState == "40001") {\n    await Task.Delay(100); // no jitter\n  }\n}`,
      },
      note: '32 workers on 10 hot rows — library backoff+jitter vs vanilla fixed delay',
    },
  ],
  mixed_rw: [
    {
      operation: 'Read with JOIN (80%)',
      library: {
        java: `// @CockroachDB with readOnly\n@CockroachDB(readOnly = true)\npublic List<OrderView> getAccountOrders(\n    String accountId) {\n  return orderRepo\n    .findByAccountIdWithDetails(\n      accountId, Limit.of(10));\n}\n\n// Programmatic\ncrdbTemplate.executeReadCommitted(() ->\n  orderRepo.findByAccountId(id));`,
        go: `pool.RawPool().Query(ctx,\n  "SELECT a.name, a.balance,\n          o.amount, o.status\n   FROM bench_accounts a\n   JOIN bench_orders o\n     ON o.account_id = a.id\n   WHERE a.id = $1\n   ORDER BY o.ts DESC\n   LIMIT 10", id)`,
        python: `crdb.execute_read_committed(lambda s:\n  s.execute(text(\n    "SELECT a.name, a.balance,\n            o.amount, o.status\n     FROM bench_accounts a\n     JOIN bench_orders o\n       ON o.account_id = a.id\n     WHERE a.id = :id\n     ORDER BY o.ts DESC\n     LIMIT 10"), {"id": uid}))`,
        dotnet: `await template\n  .ExecuteFollowerReadAsync(\n    async (conn) => {\n  await conn.ExecuteReaderAsync(\n    "SELECT a.name, a.balance,\n            o.amount, o.status\n     FROM bench_accounts a\n     JOIN bench_orders o ...\n     LIMIT 10");\n});`,
      },
      vanilla: {
        java: `jdbcTemplate.queryForList(\n  "SELECT a.name, a.balance,\n          o.amount, o.status\n   FROM bench_accounts a\n   JOIN bench_orders o\n     ON o.account_id = a.id\n   WHERE a.id = ?\n   ORDER BY o.ts DESC\n   LIMIT 10", id)`,
        go: `pool.Query(ctx,\n  "SELECT ... JOIN ... LIMIT 10", id)`,
        python: `with pool.connection() as conn:\n  conn.execute("SELECT ... JOIN ...", (uid,))`,
        dotnet: `await cmd.ExecuteReaderAsync()`,
      },
    },
  ],
  follower_reads: [
    {
      operation: 'Follower Read',
      library: {
        java: `// @FollowerRead annotation\n@FollowerRead\npublic Account getAccount(String id) {\n  return accountRepo.findById(id);\n}\n// Automatically injects:\n// AS OF SYSTEM TIME\n//   follower_read_timestamp()\n// Routes to nearest replica`,
        go: `// pgxadapter auto-injects\npool.QueryRowFollowerRead(ctx,\n  "SELECT name, balance, region\n   FROM bench_accounts\n   WHERE id = $1", id)\n\n// Internally appends:\n// AS OF SYSTEM TIME\n//   follower_read_timestamp()`,
        python: `# CockroachDBTemplate follower_read()\nwith crdb.follower_read():\n  result = crdb.execute_with_retry(\n    lambda s: s.execute(text(\n      "SELECT name, balance, region\n       FROM bench_accounts\n       WHERE id = :id"),\n      {"id": uid}).fetchone())\n# FollowerReadContext activates`,
        dotnet: `// FollowerReadMiddleware\nawait template\n  .ExecuteFollowerReadAsync(\n    async (conn) => {\n  // Middleware injects:\n  // SET TRANSACTION AS OF SYSTEM TIME\n  //   follower_read_timestamp()\n  return await conn.ExecuteReaderAsync(\n    "SELECT name, balance, region\n     FROM bench_accounts\n     WHERE id = $1", id);\n});`,
      },
      vanilla: {
        java: `// Manual SQL modification\njdbcTemplate.queryForMap(\n  "SELECT name, balance, region\n   FROM bench_accounts\n   AS OF SYSTEM TIME\n     follower_read_timestamp()\n   WHERE id = ?", id)`,
        go: `pool.QueryRow(ctx,\n  "SELECT name, balance, region\n   FROM bench_accounts\n   AS OF SYSTEM TIME\n     follower_read_timestamp()\n   WHERE id = $1", id)`,
        python: `with pool.connection() as conn:\n  conn.execute(\n    "SELECT name, balance, region\n     FROM bench_accounts\n     AS OF SYSTEM TIME '-5s'\n     WHERE id = %s", (uid,))`,
        dotnet: `await cmd.ExecuteReaderAsync();\n// Must manually prepend:\n// SET TRANSACTION AS OF SYSTEM TIME\n//   follower_read_timestamp()`,
      },
      note: 'Library auto-routes to nearest replica — no SQL changes needed',
    },
  ],
  cdc: [
    {
      operation: 'Create Changefeed + Consume',
      library: {
        java: `// @CockroachDBCDC webhook handler\n@CockroachDBCDC(\n  table = "bench_accounts",\n  operations = {INSERT, UPDATE})\npublic void onAccountChange(\n    CDCEvent<Account> event) {\n  log.info("op={} id={}",\n    event.getOperation(),\n    event.getAfter().getId());\n}\n\n// Writes via CockroachDBTemplate\ncrdbTemplate.executeWithoutResult(() ->\n  accountRepo.save(account));`,
        go: `// cdc.NewChangefeedSpec builder\nspec := cdc.NewChangefeedSpec(\n    "bench_accounts").\n  WebhookURL(url).\n  WithFormat("json").\n  Updated().Build()\n\nhandler := cdc.NewWebhookHandler(\n  func(ctx, events []Event) error {\n    for _, e := range events {\n      // e.Operation, e.Table\n    }\n    return nil\n  })`,
        python: `# @changefeed_handler decorator\n@changefeed_handler(\n    table="bench_accounts")\ndef on_change(ctx, events):\n  for e in events:\n    lag = time.now() - e.timestamp\n    record_lag(lag)`,
        dotnet: `// ChangefeedManager\nvar mgr = new ChangefeedManager(conn);\nawait mgr.CreateAsync(\n  "bench_accounts",\n  webhookUrl, format: "json");\n// Events via webhook handler`,
      },
      vanilla: {
        java: `// Raw SQL changefeed\njdbcTemplate.execute(\n  "CREATE CHANGEFEED FOR\n    bench_accounts\n    INTO 'webhook-https://...'\n    WITH format=json, updated");\n// Manual JSON parsing in webhook`,
        go: `pool.Exec(ctx,\n  "CREATE CHANGEFEED FOR\n    bench_accounts\n    INTO 'webhook-https://...'\n    WITH format=json")\n// Manual HTTP handler + JSON parse`,
        python: `conn.execute(\n  "CREATE CHANGEFEED FOR\n    bench_accounts\n    INTO 'webhook-https://...'\n    WITH format=json")\n# Manual HTTP endpoint + json.loads`,
        dotnet: `await cmd.ExecuteNonQueryAsync();\n// "CREATE CHANGEFEED FOR ..."\n// Manual webhook + JSON parsing`,
      },
    },
  ],
  vector_search: [
    {
      operation: 'Store + Search Vectors',
      library: {
        java: `// v26.2: CREATE VECTOR INDEX\nVectorIndexHelper.generateCreateVectorIndex(\n  "idx_docs", "bench_documents",\n  "embedding", DistanceMetric.COSINE);\n\n// CockroachDBVectorStore (Spring AI)\nCockroachDBVectorStore store = CockroachDBVectorStore\n  .builder(jdbcTemplate, embeddingModel)\n  .beamSize(32)\n  .createVectorIndex(true)\n  .build();`,
        go: `// v26.2: InitSchemaV26 with vector index\ntemplate.InitSchemaV26(ctx, "bench_documents",\n  "embedding", 1536,\n  &ai.VectorIndexOptions{\n    Metric: ai.Cosine,\n  })\n\n// Search with beam size\ntemplate.Search(ctx, table, col,\n  queryVec, ai.Cosine, 10, columns)`,
        python: `# v26.2: beam size tuning\nconfig = VectorSearchConfig(\n  vector_search_beam_size=32)\nvst = VectorSearchTemplate(conninfo,\n  config=config)\nvst.search("bench_documents", "embedding",\n  query_vec, "cosine", limit=10)`,
        dotnet: `// v26.2: AIOptions with beam size\nvar opts = new AIOptions {\n  BeamSize = 32,\n  CreateVectorIndex = true,\n  Metric = DistanceMetric.Cosine };\nvar vst = new VectorSearchTemplate(\n  conn, opts);\nawait vst.SearchAsync(\n  "bench_documents", "embedding",\n  queryVec, limit: 10);`,
      },
      vanilla: {
        java: `// Raw SQL with CREATE VECTOR INDEX\njdbcTemplate.execute(\n  "CREATE VECTOR INDEX idx_docs\n   ON bench_documents\n   (embedding vector_cosine_ops)");\njdbcTemplate.query(\n  "SELECT id, content,\n    embedding <=> ? AS distance\n   FROM bench_documents\n   ORDER BY embedding <=> ?\n   LIMIT 10", vector, vector)`,
        go: `pool.Exec(ctx,\n  "CREATE VECTOR INDEX idx_docs\n   ON bench_documents\n   (embedding vector_cosine_ops)")\npool.Exec(ctx,\n  "SET vector_search_beam_size = 32")\npool.Query(ctx,\n  "SELECT id, content,\n    embedding <=> $1 AS distance\n   FROM bench_documents\n   ORDER BY embedding <=> $1\n   LIMIT 10",\n  vectorToString(queryVec))`,
        python: `conn.execute(\n  "CREATE VECTOR INDEX idx_docs\n   ON bench_documents\n   (embedding vector_cosine_ops)")\nconn.execute(\n  "SET vector_search_beam_size = 32")\nconn.execute(\n  "SELECT id, content,\n    embedding <=> %s AS distance\n   FROM bench_documents\n   ORDER BY embedding <=> %s\n   LIMIT 10",\n  (vec_str, vec_str))`,
        dotnet: `await cmd.ExecuteNonQueryAsync();\n// "CREATE VECTOR INDEX idx_docs\n//  ON bench_documents\n//  (embedding vector_cosine_ops)"\nawait cmd.ExecuteNonQueryAsync();\n// "SET vector_search_beam_size = 32"\nawait cmd.ExecuteReaderAsync();\n// "SELECT id, content,\n//   embedding <=> $1 AS distance\n//  FROM bench_documents\n//  ORDER BY embedding <=> $1\n//  LIMIT 10"`,
      },
      note: 'v26.2: CREATE VECTOR INDEX + beam size tuning across all libraries',
    },
  ],
};

export function SqlPage() {
  const [activeWorkload, setActiveWorkload] = useState<WorkloadType>('crud');
  const [activeLang, setActiveLang] = useState<Lang>('go');
  const entries = WORKLOAD_SQL[activeWorkload] || [];
  const info = WORKLOAD_INFO[activeWorkload];

  return (
    <div style={{ paddingBottom: 220 }}>
      <div style={{ fontSize: 18, fontWeight: 600, marginBottom: 4 }}>SQL Catalog</div>
      <div style={{ fontSize: 13, color: '#888', marginBottom: 16 }}>
        Actual SQL and code executed by each workload — Library vs Vanilla, per language.
      </div>

      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginBottom: 12 }}>
        {WORKLOAD_TYPES.map(w => (
          <div
            key={w}
            onClick={() => setActiveWorkload(w)}
            style={{
              padding: '6px 14px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
              border: '1px solid',
              background: w === activeWorkload ? 'rgba(78,205,196,0.15)' : 'transparent',
              borderColor: w === activeWorkload ? '#4ecdc4' : '#333',
              color: w === activeWorkload ? '#4ecdc4' : '#888',
            }}
          >
            {WORKLOAD_LABELS[w]}
          </div>
        ))}
      </div>

      <div style={{ display: 'flex', gap: 4, marginBottom: 16 }}>
        {LANGS.map(l => (
          <div
            key={l.key}
            onClick={() => setActiveLang(l.key)}
            style={{
              padding: '5px 14px', fontSize: 12, borderRadius: 4, cursor: 'pointer',
              border: '1px solid',
              background: l.key === activeLang ? `${LANGUAGE_COLORS[l.key]}22` : 'transparent',
              borderColor: l.key === activeLang ? LANGUAGE_COLORS[l.key] : '#333',
              color: l.key === activeLang ? LANGUAGE_COLORS[l.key] : '#666',
              fontWeight: l.key === activeLang ? 600 : 400,
            }}
          >
            {l.label}
          </div>
        ))}
      </div>

      {info && (
        <div style={{
          padding: 10, marginBottom: 14, fontSize: 12, lineHeight: 1.6, color: '#aaa',
          background: 'rgba(78,205,196,0.03)', border: '1px solid rgba(78,205,196,0.15)', borderRadius: 6,
        }}>
          <strong style={{ color: '#4ecdc4' }}>{WORKLOAD_LABELS[activeWorkload]}:</strong> {info.description}
        </div>
      )}

      {entries.map((entry, i) => (
        <div key={i} style={{
          background: 'var(--bg-card)', border: '1px solid var(--border)',
          borderRadius: 8, marginBottom: 12, overflow: 'hidden',
        }}>
          <div style={{
            padding: '8px 14px', fontSize: 13, fontWeight: 600,
            borderBottom: '1px solid var(--border)',
            display: 'flex', justifyContent: 'space-between', alignItems: 'center',
          }}>
            <span>{entry.operation}</span>
            {entry.note && <span style={{ fontSize: 11, fontWeight: 400, color: '#888' }}>{entry.note}</span>}
          </div>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr' }}>
            <div style={{
              padding: 12, borderRight: '1px solid var(--border)',
              fontFamily: 'monospace', fontSize: 11, lineHeight: 1.5, whiteSpace: 'pre-wrap', overflowX: 'auto',
            }}>
              <div style={{
                fontSize: 10, textTransform: 'uppercase', fontWeight: 600,
                color: LANGUAGE_COLORS[activeLang], marginBottom: 6, fontFamily: 'sans-serif',
              }}>
                Library ({LANGS.find(l => l.key === activeLang)?.label})
              </div>
              <CodeBlock code={entry.library[activeLang]} lang={activeLang} />
            </div>
            <div style={{
              padding: 12, fontFamily: 'monospace', fontSize: 11, lineHeight: 1.5,
              whiteSpace: 'pre-wrap', overflowX: 'auto',
            }}>
              <div style={{
                fontSize: 10, textTransform: 'uppercase', fontWeight: 600,
                color: '#888', marginBottom: 6, fontFamily: 'sans-serif',
              }}>
                Vanilla ({LANGS.find(l => l.key === activeLang)?.label})
              </div>
              <CodeBlock code={entry.vanilla[activeLang]} lang={activeLang} />
            </div>
          </div>
        </div>
      ))}

      <div style={{
        padding: 16, marginTop: 20,
        background: 'rgba(192,132,252,0.05)', border: '1px dashed rgba(192,132,252,0.3)',
        borderRadius: 8, fontSize: 12, color: '#888', textAlign: 'center',
      }}>
        Coming soon: Custom scenario editor — define your own SQL workloads in JSON format, set per-operation concurrency, and run them as benchmarks.
      </div>
    </div>
  );
}
