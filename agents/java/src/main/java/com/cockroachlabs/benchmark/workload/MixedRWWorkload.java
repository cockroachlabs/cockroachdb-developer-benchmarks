package com.cockroachlabs.benchmark.workload;

import com.cockroachlabs.benchmark.protocol.WorkloadRequest;
import com.cockroachlabs.data.template.CockroachDBTemplate;
import org.springframework.jdbc.core.JdbcTemplate;

import javax.sql.DataSource;
import java.math.BigDecimal;
import java.sql.*;
import java.util.UUID;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.ThreadLocalRandom;
import java.util.concurrent.atomic.AtomicBoolean;

public class MixedRWWorkload implements WorkloadRunner {

    private final DataSource dataSource;
    private final String mode;
    private final CockroachDBTemplate template;
    private final MetricsRecorder metrics = new MetricsRecorder();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ExecutorService executor;

    public MixedRWWorkload(DataSource dataSource, String mode, CockroachDBTemplate template) {
        this.dataSource = dataSource;
        this.mode = mode;
        this.template = template;
    }

    @Override
    public void start(WorkloadRequest.WorkloadConfig config) {
        if (running.getAndSet(true)) return;
        metrics.reset();
        metrics.setMode(mode);
        metrics.setWorkload("mixed_rw");
        int concurrency = config.concurrency() > 0 ? config.concurrency() : 8;
        executor = Executors.newVirtualThreadPerTaskExecutor();
        for (int i = 0; i < concurrency; i++) {
            executor.submit(() -> runLoop(config));
        }
    }

    private void runLoop(WorkloadRequest.WorkloadConfig config) {
        long deadline = System.currentTimeMillis() + (config.durationSecs() * 1000L);
        if ("library".equals(mode)) {
            runLibraryLoop(deadline);
        } else {
            runVanillaLoop(deadline);
        }
    }

    private void runLibraryLoop(long deadline) {
        JdbcTemplate jdbc = new JdbcTemplate(dataSource);
        while (running.get() && System.currentTimeMillis() < deadline) {
            try {
                long start = System.nanoTime();
                boolean isRead = ThreadLocalRandom.current().nextInt(100) < 80;

                if (isRead) {
                    template.execute(() ->
                        jdbc.queryForList(
                                "SELECT a.id, a.balance, a.name FROM bench_accounts a " +
                                "LEFT JOIN bench_orders o ON o.account_id = a.id " +
                                "ORDER BY a.updated_at DESC LIMIT 20")
                    );
                } else {
                    template.execute(() -> {
                        var accounts = jdbc.queryForList(
                                "SELECT id FROM bench_accounts ORDER BY random() LIMIT 1");
                        if (!accounts.isEmpty()) {
                            String accountId = accounts.get(0).get("id").toString();
                            BigDecimal amount = BigDecimal.valueOf(ThreadLocalRandom.current().nextDouble(10, 500));
                            jdbc.update("INSERT INTO bench_orders (id, account_id, amount, status) VALUES (?::UUID, ?::UUID, ?, 'pending')",
                                    UUID.randomUUID().toString(), accountId, amount);
                        }
                        return null;
                    });
                }

                long elapsed = (System.nanoTime() - start) / 1000;
                metrics.recordOp(elapsed);
            } catch (Exception e) {
                metrics.recordError();
            }
        }
    }

    private void runVanillaLoop(long deadline) {
        while (running.get() && System.currentTimeMillis() < deadline) {
            try (Connection conn = dataSource.getConnection()) {
                long start = System.nanoTime();
                boolean isRead = ThreadLocalRandom.current().nextInt(100) < 80;

                if (isRead) {
                    try (PreparedStatement ps = conn.prepareStatement(
                            "SELECT a.id, a.balance, a.name FROM bench_accounts a " +
                            "LEFT JOIN bench_orders o ON o.account_id = a.id " +
                            "ORDER BY a.updated_at DESC LIMIT 20")) {
                        ps.executeQuery().close();
                    }
                } else {
                    String accountId = null;
                    try (PreparedStatement ps = conn.prepareStatement(
                            "SELECT id FROM bench_accounts ORDER BY random() LIMIT 1")) {
                        ResultSet rs = ps.executeQuery();
                        if (rs.next()) accountId = rs.getString(1);
                    }
                    if (accountId != null) {
                        try (PreparedStatement ps = conn.prepareStatement(
                                "INSERT INTO bench_orders (id, account_id, amount, status) VALUES (?::UUID, ?::UUID, ?, 'pending')")) {
                            ps.setString(1, UUID.randomUUID().toString());
                            ps.setString(2, accountId);
                            ps.setBigDecimal(3, BigDecimal.valueOf(ThreadLocalRandom.current().nextDouble(10, 500)));
                            ps.executeUpdate();
                        }
                    }
                }

                long elapsed = (System.nanoTime() - start) / 1000;
                metrics.recordOp(elapsed);
            } catch (Exception e) {
                metrics.recordError();
            }
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
