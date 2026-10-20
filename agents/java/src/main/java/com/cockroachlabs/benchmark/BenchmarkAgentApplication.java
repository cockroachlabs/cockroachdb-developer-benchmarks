package com.cockroachlabs.benchmark;

import org.springframework.boot.SpringApplication;
import org.springframework.boot.autoconfigure.SpringBootApplication;

@SpringBootApplication
public class BenchmarkAgentApplication {

    public static void main(String[] args) {
        SpringApplication.run(BenchmarkAgentApplication.class, args);
    }
}
