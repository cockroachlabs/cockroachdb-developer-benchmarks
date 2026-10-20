FROM gradle:8-jdk21 AS builder
WORKDIR /app

COPY cockroachdb-spring-boot-starter/ /cockroachdb-spring-boot-starter/

COPY build.gradle settings.gradle ./
RUN sed -i "s|/Users/felipeg/Progs/AI/cockroachdb-spring-boot-starter|/cockroachdb-spring-boot-starter|g" settings.gradle
RUN gradle dependencies --no-daemon || true

COPY src src
RUN gradle bootJar --no-daemon -x test

FROM eclipse-temurin:21-jre
WORKDIR /app
COPY --from=builder /app/build/libs/*.jar app.jar
ENV AGENT_PORT=7001
ENV AGENT_MODE=library
EXPOSE ${AGENT_PORT}
ENTRYPOINT ["java", "-jar", "app.jar"]
