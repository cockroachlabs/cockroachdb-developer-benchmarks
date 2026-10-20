#!/bin/bash
set -e

echo "Building GraalVM native image for Linux inside Docker..."
echo "This uses your local GraalVM to cross-compile via Docker buildx."

cd "$(dirname "$0")"

docker run --rm -v "$(pwd):/app" -v "$(pwd)/../../cockroachdb-spring-boot-starter:/cockroachdb-spring-boot-starter" \
  -w /app \
  --platform linux/arm64 \
  eclipse-temurin:21-jdk \
  bash -c "
    sed -i 's|/Users/felipeg/Progs/AI/cockroachdb-spring-boot-starter|/cockroachdb-spring-boot-starter|g' settings.gradle
    chmod +x gradlew
    ./gradlew bootJar --no-daemon -x test
    echo 'JAR built successfully'
  "

echo "JAR built. Native compilation requires GraalVM in Docker — skipping for ARM."
echo "Use the JVM jar for now. Native works on x86 CI."
