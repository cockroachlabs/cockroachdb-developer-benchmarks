package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.benchmark.protocol.WorkloadRequest;
import com.cockroachlabs.data.template.CockroachDBTemplate;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;
import java.math.BigDecimal;
import java.sql.*;
import java.util.List;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicBoolean;

public class RetryStormWorkload implements WorkloadRunner {

    private final DataSource dataSource;
    private final String mode;
    private final CockroachDBTemplate template;
    private final MetricsRecorder metrics = new MetricsRecorder();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ExecutorService executor;

    public RetryStormWorkload(DataSource dataSource, String mode, CockroachDBTemplate template) {
        this.dataSource = dataSource;
        this.mode = mode;
        this.template = template;
    }

    @Override
    public void start(WorkloadRequest.WorkloadConfig config) {
        if (running.getAndSet(true)) return;
        metrics.reset();
        metrics.setMode(mode);
        metrics.setWorkload("retry_storm");
        int concurrency = config.concurrency() > 0 ? config.concurrency() : 32;
        int hotRows = config.hotRows() > 0 ? config.hotRows() : 10;
        executor = Executors.newVirtualThreadPerTaskExecutor();
        List<String> hotIds = loadHotRowIds(hotRows);
        for (int i = 0; i < concurrency; i++) {
            executor.submit(() -> runLoop(config, hotIds));
        }
    }

    private List<String> loadHotRowIds(int count) {
        try (Connection conn = dataSource.getConnection();
             PreparedStatement ps = conn.prepareStatement(
                     "SELECT id FROM bench_accounts LIMIT ?")) {
            ps.setInt(1, count);
            ResultSet rs = ps.executeQuery();
            var ids = new java.util.ArrayList<String>();
            while (rs.next()) ids.add(rs.getString(1));
            return ids;
        } catch (SQLException e) {
            throw new RuntimeException("Failed to load hot rows", e);
        }
    }

    private void runLoop(WorkloadRequest.WorkloadConfig config, List<String> hotIds) {
        long deadline = System.currentTimeMillis() + (config.durationSecs() * 1000L);
        while (running.get() && System.currentTimeMillis() < deadline) {
            String fromId = hotIds.get(ThreadLocalRandom.current().nextInt(hotIds.size()));
            String toId = hotIds.get(ThreadLocalRandom.current().nextInt(hotIds.size()));
            if (fromId.equals(toId)) continue;
            BigDecimal amount = BigDecimal.valueOf(ThreadLocalRandom.current().nextDouble(1, 100));

            if ("library".equals(mode)) {
                executeWithLibraryRetry(fromId, toId, amount);
            } else {
                executeWithVanillaRetry(fromId, toId, amount);
            }
        }
    }

    private void executeWithLibraryRetry(String fromId, String toId, BigDecimal amount) {
        long start = System.nanoTime();
        try {
            JdbcTemplate jdbc = new JdbcTemplate(dataSource);
            template.execute(() -> {
                jdbc.update("UPDATE bench_accounts SET balance = balance - ?, updated_at = now() WHERE id = ?::UUID",
                        amount, fromId);
                jdbc.update("UPDATE bench_accounts SET balance = balance + ?, updated_at = now() WHERE id = ?::UUID",
                        amount, toId);
                return null;
            });
            long elapsed = (System.nanoTime() - start) / 1000;
            metrics.recordOp(elapsed);
        } catch (Exception e) {
            metrics.recordError();
        }
    }

    private void executeWithVanillaRetry(String fromId, String toId, BigDecimal amount) {
        int maxRetries = 10;
        for (int attempt = 0; attempt <= maxRetries; attempt++) {
            long start = System.nanoTime();
            try (Connection conn = dataSource.getConnection()) {
                conn.setAutoCommit(false);
                try (Statement st = conn.createStatement()) {
                    st.execute("BEGIN");
                    doTransfer(conn, fromId, toId, amount);
                    st.execute("COMMIT");
                    long elapsed = (System.nanoTime() - start) / 1000;
                    metrics.recordOp(elapsed);
                    if (attempt > 0) metrics.recordRetry(true);
                    return;
                } catch (SQLException e) {
                    try (Statement st = conn.createStatement()) { st.execute("ROLLBACK"); } catch (SQLException ignored) {}
                    if ("40001".equals(e.getSQLState()) && attempt < maxRetries) {
                        metrics.recordRetry(false);
                        Thread.sleep((long) (Math.pow(2, attempt) * 5));
                        continue;
                    }
                    throw e;
                }
            } catch (Exception e) {
                metrics.recordError();
                return;
            }
        }
    }

    private void doTransfer(Connection conn, String fromId, String toId, BigDecimal amount) throws SQLException {
        try (PreparedStatement ps = conn.prepareStatement(
                "UPDATE bench_accounts SET balance = balance - ?, updated_at = now() WHERE id = ?::UUID")) {
            ps.setBigDecimal(1, amount);
            ps.setString(2, fromId);
            ps.executeUpdate();
        }
        try (PreparedStatement ps = conn.prepareStatement(
                "UPDATE bench_accounts SET balance = balance + ?, updated_at = now() WHERE id = ?::UUID")) {
            ps.setBigDecimal(1, amount);
            ps.setString(2, toId);
            ps.executeUpdate();
        }
    }

    @Override
    public void stop() {
        running.set(false);
        if (executor != null) executor.shutdown();
    }

    @Override
    public boolean isRunning() { return running.get(); }

    @Override
    public MetricsRecorder getMetrics() { return metrics; }
}
