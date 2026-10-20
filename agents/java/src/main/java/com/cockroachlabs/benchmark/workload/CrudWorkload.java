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

public class CrudWorkload implements WorkloadRunner {

    private final DataSource dataSource;
    private final String mode;
    private final CockroachDBTemplate template;
    private final MetricsRecorder metrics = new MetricsRecorder();
    private final AtomicBoolean running = new AtomicBoolean(false);
    private ExecutorService executor;

    public CrudWorkload(DataSource dataSource, String mode, CockroachDBTemplate template) {
        this.dataSource = dataSource;
        this.mode = mode;
        this.template = template;
    }

    @Override
    public void start(WorkloadRequest.WorkloadConfig config) {
        if (running.getAndSet(true)) return;
        metrics.reset();
        metrics.setMode(mode);
        metrics.setWorkload("crud");
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
                int op = ThreadLocalRandom.current().nextInt(4);
                String id = UUID.randomUUID().toString();
                BigDecimal balance = BigDecimal.valueOf(1000 + ThreadLocalRandom.current().nextDouble() * 9000);
                String name = "acct-" + ThreadLocalRandom.current().nextInt(100000);
                String region = pickRegion();

                template.execute(() -> {
                    switch (op) {
                        case 0 -> jdbc.update(
                                "INSERT INTO bench_accounts (id, balance, name, region) VALUES (?::UUID, ?, ?, ?)",
                                id, balance, name, region);
                        case 1 -> jdbc.queryForList(
                                "SELECT name, balance FROM bench_accounts WHERE id = ?::UUID",
                                UUID.randomUUID().toString());
                        case 2 -> jdbc.update(
                                "UPDATE bench_accounts SET balance = balance + ?, updated_at = now() WHERE id = ?::UUID",
                                BigDecimal.valueOf(ThreadLocalRandom.current().nextDouble() * 100 - 50),
                                UUID.randomUUID().toString());
                        case 3 -> jdbc.update(
                                "DELETE FROM bench_accounts WHERE id = ?::UUID",
                                UUID.randomUUID().toString());
                    }
                    return null;
                });

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
                int op = ThreadLocalRandom.current().nextInt(4);

                switch (op) {
                    case 0 -> {
                        try (PreparedStatement ps = conn.prepareStatement(
                                "INSERT INTO bench_accounts (id, balance, name, region) VALUES (?::UUID, ?, ?, ?)")) {
                            ps.setString(1, UUID.randomUUID().toString());
                            ps.setBigDecimal(2, BigDecimal.valueOf(1000 + ThreadLocalRandom.current().nextDouble() * 9000));
                            ps.setString(3, "acct-" + ThreadLocalRandom.current().nextInt(100000));
                            ps.setString(4, pickRegion());
                            ps.executeUpdate();
                        }
                    }
                    case 1 -> {
                        try (PreparedStatement ps = conn.prepareStatement(
                                "SELECT name, balance FROM bench_accounts WHERE id = ?::UUID")) {
                            ps.setString(1, UUID.randomUUID().toString());
                            try (ResultSet rs = ps.executeQuery()) {
                                while (rs.next()) { }
                            }
                        }
                    }
                    case 2 -> {
                        try (PreparedStatement ps = conn.prepareStatement(
                                "UPDATE bench_accounts SET balance = balance + ?, updated_at = now() WHERE id = ?::UUID")) {
                            ps.setBigDecimal(1, BigDecimal.valueOf(ThreadLocalRandom.current().nextDouble() * 100 - 50));
                            ps.setString(2, UUID.randomUUID().toString());
                            ps.executeUpdate();
                        }
                    }
                    case 3 -> {
                        try (PreparedStatement ps = conn.prepareStatement(
                                "DELETE FROM bench_accounts WHERE id = ?::UUID")) {
                            ps.setString(1, UUID.randomUUID().toString());
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

    private static String pickRegion() {
        return switch (ThreadLocalRandom.current().nextInt(3)) {
            case 0 -> "us-east";
            case 1 -> "us-west";
            default -> "eu-west";
        };
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
