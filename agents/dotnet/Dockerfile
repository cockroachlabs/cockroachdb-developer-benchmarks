FROM mcr.microsoft.com/dotnet/sdk:10.0 AS build
WORKDIR /src

COPY BenchmarkAgent.csproj Directory.Build.props ./
COPY cockroachdb-dotnet-library/ /cockroachdb-dotnet-library/

RUN dotnet restore BenchmarkAgent.csproj \
    -p:CockroachDBLibraryRoot=/cockroachdb-dotnet-library \
    -p:TreatWarningsAsErrors=false

COPY src/ ./src/
COPY Program.cs ./

RUN dotnet publish BenchmarkAgent.csproj \
    -c Release -o /app \
    -p:CockroachDBLibraryRoot=/cockroachdb-dotnet-library \
    -p:TreatWarningsAsErrors=false

FROM mcr.microsoft.com/dotnet/aspnet:10.0
WORKDIR /app
COPY --from=build /app .

ENV AGENT_MODE=vanilla
ENV AGENT_PORT=7008
EXPOSE ${AGENT_PORT}

ENTRYPOINT ["dotnet", "BenchmarkAgent.dll"]
