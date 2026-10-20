package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"sort"
	"time"

	"github.com/cockroachlabs/cockroachdb-developer-benchmarks/controller/protocol"
)

type agentKey struct {
	Language string
	Mode     string
}

type AgentSummary struct {
	Language     string
	Mode         string
	Color        string
	AvgOpsPerSec float64
	StdDev       float64
	CI95Lower    float64
	CI95Upper    float64
	Iterations   int
	P50Display   string
	P95Display   string
	P99Display   string
	TotalErrors  int64
	TotalRetries int64
}

type OpsDataset struct {
	Label       string
	DataJSON    template.JS
	Color       string
	BorderWidth int
}

type WorkloadAgentRow struct {
	Language     string
	Mode         string
	Color        string
	AvgOpsPerSec float64
	StdDev       float64
	CI95Lower    float64
	CI95Upper    float64
	Iterations   int
	P50Display   string
	P95Display   string
	P99Display   string
	TotalErrors  int64
	TotalRetries int64
	ErrorSamples []protocol.ErrorSample
}

type WorkloadSection struct {
	Name             string
	Description      string
	WhatWeMeasure    string
	LibraryAdvantage string
	WhatToExpect     string
	TimeLabelsJSON   template.JS
	OpsDatasets      []OpsDataset
	LatencyLabelsJSON template.JS
	LatencyColorsJSON template.JS
	P50DataJSON      template.JS
	P95DataJSON      template.JS
	P99DataJSON      template.JS
	AgentRows        []WorkloadAgentRow
}

type ChaosTimelineEntry struct {
	TimeDisplay string
	Scenario    string
	Action      string
	Target      string
}

type KeyInsight struct {
	Icon        string
	Title       string
	Description string
	ColorHex    string
	ColorRGB    string
}

type ProConItem struct {
	Text string
}

type LangProsCons struct {
	Language string
	ColorHex string
	Pros     []ProConItem
	Cons     []ProConItem
}

type RegressionAlert struct {
	Agent         string
	Workload      string
	Metric        string
	PreviousValue float64
	CurrentValue  float64
	ChangePct     float64
	IsRegression  bool
}

type ExecutiveSummary struct {
	FastestAgent   string
	FastestOps     float64
	BestLibrary    string
	BestLibraryOps float64
	OverallVerdict string
	TopFindings    []string
}

type reportData struct {
	Executive              ExecutiveSummary
	RegressionAlerts       []RegressionAlert
	HasRegressions         bool
	KeyInsights            []KeyInsight
	ProsCons               []LangProsCons
	Run                   protocol.BenchmarkRun
	RunTimestamp           string
	WorkloadCount          int
	AgentCount             int
	AgentSummaries         []AgentSummary
	Workloads              []string
	WorkloadSections       []WorkloadSection
	HasChaos               bool
	ChaosTimeline          []ChaosTimelineEntry
	ComparisonLabelsJSON   template.JS
	ComparisonDatasetsJSON template.JS
	GeneratedAt            string
}

var langColorMap = map[string]string{
	"java":        "#4ecdc4",
	"java-native": "#2ecc71",
	"go":          "#ff6b6b",
	"python":      "#ffd93d",
	"dotnet":      "#c084fc",
}

var agentColorMap = map[string]string{
	"java/library":        "#4ecdc4",
	"java/vanilla":        "#ff9f43",
	"java-native/library": "#2ecc71",
	"java-native/vanilla": "#0abde3",
	"go/library":          "#ff6b6b",
	"go/vanilla":          "#feca57",
	"python/library":      "#a29bfe",
	"python/vanilla":      "#fd79a8",
	"dotnet/library":      "#00cec9",
	"dotnet/vanilla":      "#e17055",
}

func GenerateComparisonHTML(runs []protocol.BenchmarkRun, allMetrics [][]protocol.MetricSnapshot) ([]byte, error) {
	type compRunData struct {
		RunID     string
		Timestamp string
		Topology  string
		Duration  int
		Agents    int
		Summaries []AgentSummary
	}

	var runDataList []compRunData
	for i, run := range runs {
		rd := compRunData{
			RunID:     run.ID,
			Timestamp: run.Timestamp.Format("2006-01-02 15:04:05 MST"),
			Topology:  string(run.Topology),
			Duration:  run.Duration,
			Agents:    len(run.Agents),
		}
		if i < len(allMetrics) {
			rd.Summaries = buildAgentSummaries(allMetrics[i])
		}
		runDataList = append(runDataList, rd)
	}

	type compData struct {
		Runs        []compRunData
		RunCount    int
		GeneratedAt string
	}

	data := compData{
		Runs:        runDataList,
		RunCount:    len(runs),
		GeneratedAt: time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	tmpl, err := template.New("comparison").Parse(comparisonTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse comparison template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute comparison template: %w", err)
	}

	return buf.Bytes(), nil
}

var comparisonTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>CockroachDB Benchmark Comparison — {{.RunCount}} Runs</title>
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{--bg:#0a0a0a;--surface:#141414;--surface2:#1e1e1e;--border:#2a2a2a;--text:#e0e0e0;--text-dim:#888;--accent:#6366f1}
body{background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,monospace;line-height:1.6;padding:20px}
.container{max-width:1400px;margin:0 auto}
h1{font-size:1.6rem;margin-bottom:4px}
h2{font-size:1.2rem;margin:32px 0 16px;border-bottom:1px solid var(--border);padding-bottom:8px}
.header{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:24px;margin-bottom:24px}
.run-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(400px,1fr));gap:16px;margin-bottom:24px}
.run-card{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:16px}
.run-card h3{font-size:0.95rem;margin-bottom:8px;color:var(--accent)}
.meta{font-size:0.8rem;color:var(--text-dim);margin-bottom:12px}
table{width:100%;border-collapse:collapse;font-size:0.85rem;margin-top:8px}
th{text-align:left;padding:6px 10px;border-bottom:1px solid var(--border);color:var(--text-dim);font-weight:500;font-size:0.75rem;text-transform:uppercase}
td{padding:6px 10px;border-bottom:1px solid var(--border)}
.color-dot{display:inline-block;width:8px;height:8px;border-radius:50%;margin-right:6px}
.footer{text-align:center;color:var(--text-dim);font-size:0.75rem;margin-top:32px;padding-top:16px;border-top:1px solid var(--border)}
</style>
</head>
<body>
<div class="container">
<div class="header">
<h1>Benchmark Comparison Report</h1>
<p style="color:var(--text-dim);font-size:0.9rem">Comparing {{.RunCount}} benchmark runs</p>
</div>

<h2>Run Summaries</h2>
<div class="run-grid">
{{range $i, $run := .Runs}}
<div class="run-card">
<h3>Run {{$i}}: {{$run.RunID}}</h3>
<div class="meta">{{$run.Timestamp}} | {{$run.Topology}} | {{$run.Duration}}s | {{$run.Agents}} agents</div>
<table>
<thead><tr><th>Agent</th><th>Avg ops/s</th><th>P50</th><th>P95</th><th>P99</th><th>Errors</th></tr></thead>
<tbody>
{{range $run.Summaries}}
<tr>
<td><span class="color-dot" style="background:{{.Color}}"></span>{{.Language}} {{.Mode}}</td>
<td>{{printf "%.0f" .AvgOpsPerSec}}</td>
<td>{{.P50Display}}</td>
<td>{{.P95Display}}</td>
<td>{{.P99Display}}</td>
<td>{{.TotalErrors}}</td>
</tr>
{{end}}
</tbody>
</table>
</div>
{{end}}
</div>

<div class="footer">Generated {{.GeneratedAt}} | CockroachDB Developer Benchmarks</div>
</div>
</body>
</html>`

func DetectRegressions(current, previous []AgentSummary) []RegressionAlert {
	prevMap := map[string]AgentSummary{}
	for _, s := range previous {
		prevMap[s.Language+"/"+s.Mode] = s
	}
	var alerts []RegressionAlert
	for _, curr := range current {
		key := curr.Language + "/" + curr.Mode
		prev, ok := prevMap[key]
		if !ok || prev.AvgOpsPerSec <= 0 {
			continue
		}
		changePct := ((curr.AvgOpsPerSec - prev.AvgOpsPerSec) / prev.AvgOpsPerSec) * 100
		alerts = append(alerts, RegressionAlert{
			Agent:         key,
			Workload:      "overall",
			Metric:        "ops_per_sec",
			PreviousValue: prev.AvgOpsPerSec,
			CurrentValue:  curr.AvgOpsPerSec,
			ChangePct:     math.Round(changePct*10) / 10,
			IsRegression:  changePct < -10,
		})
	}
	return alerts
}

func buildExecutiveSummary(summaries []AgentSummary, metrics []protocol.MetricSnapshot) ExecutiveSummary {
	exec := ExecutiveSummary{}

	for _, s := range summaries {
		if s.AvgOpsPerSec > exec.FastestOps {
			exec.FastestOps = s.AvgOpsPerSec
			exec.FastestAgent = s.Language + " " + s.Mode
		}
	}

	bestLibOverhead := math.MaxFloat64
	for _, lang := range []string{"java", "go", "python", "dotnet"} {
		var libOps, vanOps float64
		var libFound, vanFound bool
		for _, s := range summaries {
			if s.Language == lang && s.Mode == "library" {
				libOps = s.AvgOpsPerSec
				libFound = true
			}
			if s.Language == lang && s.Mode == "vanilla" {
				vanOps = s.AvgOpsPerSec
				vanFound = true
			}
		}
		if libFound && vanFound && vanOps > 0 {
			overhead := (vanOps - libOps) / vanOps * 100
			if overhead < bestLibOverhead {
				bestLibOverhead = overhead
				exec.BestLibrary = lang
				exec.BestLibraryOps = libOps
			}
		}
	}

	type errAgg struct{ lib, van int64 }
	errByLang := map[string]*errAgg{}
	for _, m := range metrics {
		if m.Workload != "retry_storm" {
			continue
		}
		a, ok := errByLang[m.Language]
		if !ok {
			a = &errAgg{}
			errByLang[m.Language] = a
		}
		if m.Mode == "library" {
			a.lib += m.Errors
		} else {
			a.van += m.Errors
		}
	}

	overheadStr := "minimal"
	if bestLibOverhead > 5 {
		overheadStr = fmt.Sprintf("%.0f%%", bestLibOverhead)
	}

	totalErrRatio := 0.0
	ratioCount := 0
	for _, a := range errByLang {
		if a.van > 0 {
			totalErrRatio += float64(a.van-a.lib) / float64(a.van) * 100
			ratioCount++
		}
	}
	failReduction := "significant"
	if ratioCount > 0 {
		failReduction = fmt.Sprintf("%.0f%%", totalErrRatio/float64(ratioCount))
	}

	exec.OverallVerdict = fmt.Sprintf(
		"Libraries provide %s overhead on CRUD but prevent %s of failures under contention.",
		overheadStr, failReduction)

	if exec.FastestAgent != "" {
		exec.TopFindings = append(exec.TopFindings,
			fmt.Sprintf("Fastest overall: %s at %.0f ops/sec", exec.FastestAgent, exec.FastestOps))
	}
	if exec.BestLibrary != "" {
		exec.TopFindings = append(exec.TopFindings,
			fmt.Sprintf("Best library performance: %s at %.0f ops/sec", exec.BestLibrary, exec.BestLibraryOps))
	}
	for lang, a := range errByLang {
		if a.van > a.lib*2 {
			ratio := float64(a.van) / math.Max(float64(a.lib), 1)
			exec.TopFindings = append(exec.TopFindings,
				fmt.Sprintf("%s library reduces contention errors by %.0fx vs vanilla", lang, ratio))
		}
	}
	if len(exec.TopFindings) > 3 {
		exec.TopFindings = exec.TopFindings[:3]
	}

	return exec
}

func GenerateHTML(run protocol.BenchmarkRun, metrics []protocol.MetricSnapshot, chaosEvents []protocol.ChaosEvent, previousMetrics ...[]protocol.MetricSnapshot) ([]byte, error) {
	data := buildReportData(run, metrics, chaosEvents)

	if len(previousMetrics) > 0 && len(previousMetrics[0]) > 0 {
		prevSummaries := buildAgentSummaries(previousMetrics[0])
		data.RegressionAlerts = DetectRegressions(data.AgentSummaries, prevSummaries)
		data.HasRegressions = len(data.RegressionAlerts) > 0
	}

	tmpl, err := template.New("report").Parse(htmlTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}

	return buf.Bytes(), nil
}

func buildReportData(run protocol.BenchmarkRun, metrics []protocol.MetricSnapshot, chaosEvents []protocol.ChaosEvent) reportData {
	workloadNames := make([]string, len(run.Workloads))
	for i, w := range run.Workloads {
		workloadNames[i] = string(w)
	}

	data := reportData{
		Run:            run,
		RunTimestamp:    run.Timestamp.Format("2006-01-02 15:04:05 MST"),
		WorkloadCount:  len(run.Workloads),
		AgentCount:     len(run.Agents),
		Workloads:      workloadNames,
		HasChaos:       len(chaosEvents) > 0,
		GeneratedAt:    time.Now().Format("2006-01-02 15:04:05 MST"),
	}

	data.AgentSummaries = buildAgentSummaries(metrics)
	data.Executive = buildExecutiveSummary(data.AgentSummaries, metrics)
	data.WorkloadSections = buildWorkloadSections(workloadNames, metrics)
	data.ChaosTimeline = buildChaosTimeline(chaosEvents, run.Timestamp)

	compLabels, compDatasets := buildComparisonData(workloadNames, metrics)
	data.ComparisonLabelsJSON = toJS(compLabels)
	data.ComparisonDatasetsJSON = template.JS(compDatasets)

	data.KeyInsights = buildKeyInsights(metrics, workloadNames)
	data.ProsCons = buildProsCons(metrics)

	return data
}

func buildProsCons(metrics []protocol.MetricSnapshot) []LangProsCons {
	type stats struct {
		ops     float64
		errors  int64
		retries int64
		count   int
	}
	byKey := map[string]*stats{}
	for _, m := range metrics {
		if m.OpsPerSec <= 0 {
			continue
		}
		key := m.Language + "/" + m.Mode + "/" + m.Workload
		s, ok := byKey[key]
		if !ok {
			s = &stats{}
			byKey[key] = s
		}
		s.ops += m.OpsPerSec
		s.errors = m.Errors
		s.retries = m.Retries
		s.count++
	}

	avg := func(key string) float64 {
		s, ok := byKey[key]
		if !ok || s.count == 0 {
			return 0
		}
		return s.ops / float64(s.count)
	}

	errs := func(key string) int64 {
		s, ok := byKey[key]
		if !ok {
			return 0
		}
		return s.errors
	}

	langInfo := []struct {
		lang  string
		color string
	}{
		{"java", "4ecdc4"},
		{"go", "ff6b6b"},
		{"python", "ffd93d"},
		{"dotnet", "c084fc"},
	}

	var result []LangProsCons
	for _, li := range langInfo {
		lang := li.lang
		pc := LangProsCons{Language: lang, ColorHex: li.color}

		libCrud := avg(lang + "/library/crud")
		vanCrud := avg(lang + "/vanilla/crud")
		libRetry := avg(lang + "/library/retry_storm")
		vanRetry := avg(lang + "/vanilla/retry_storm")
		libErrs := errs(lang + "/library/retry_storm")
		vanErrs := errs(lang + "/vanilla/retry_storm")

		if libCrud > 0 || vanCrud > 0 {
			if vanCrud > 0 && libCrud > 0 {
				overhead := ((vanCrud - libCrud) / vanCrud) * 100
				if overhead < 5 {
					pc.Pros = append(pc.Pros, ProConItem{fmt.Sprintf("Minimal CRUD overhead (%.1f%%)", overhead)})
				} else if overhead < 0 {
					pc.Pros = append(pc.Pros, ProConItem{fmt.Sprintf("Library is %.1f%% FASTER on CRUD", -overhead)})
				} else {
					pc.Cons = append(pc.Cons, ProConItem{fmt.Sprintf("%.1f%% CRUD overhead vs vanilla", overhead)})
				}
			}
		}

		if libRetry > 0 || vanRetry > 0 {
			if vanErrs > libErrs*2 {
				ratio := float64(vanErrs) / math.Max(float64(libErrs), 1)
				pc.Pros = append(pc.Pros, ProConItem{fmt.Sprintf("%.0fx fewer errors under contention (retry storm)", ratio)})
			} else if libErrs > vanErrs*2 {
				pc.Cons = append(pc.Cons, ProConItem{"More errors than vanilla under contention — retry strategy needs tuning"})
			}
			if libRetry > vanRetry*1.1 {
				pc.Pros = append(pc.Pros, ProConItem{fmt.Sprintf("%.0f%% higher throughput under contention", ((libRetry-vanRetry)/vanRetry)*100)})
			}
		}

		pc.Pros = append(pc.Pros, ProConItem{"Automatic retry with exponential backoff + jitter"})
		pc.Pros = append(pc.Pros, ProConItem{"Built-in connection pool optimization for CockroachDB"})
		pc.Pros = append(pc.Pros, ProConItem{"Follower read routing (multi-node clusters)"})
		pc.Pros = append(pc.Pros, ProConItem{"Circuit breaker for failure resilience"})

		if libCrud == 0 && vanCrud > 0 {
			pc.Cons = append(pc.Cons, ProConItem{"Library agent did not produce data — check configuration"})
		}

		hasData := false
		for k := range byKey {
			if len(k) > len(lang) && k[:len(lang)] == lang {
				hasData = true
				break
			}
		}
		if hasData {
			result = append(result, pc)
		}
	}
	return result
}

func buildKeyInsights(metrics []protocol.MetricSnapshot, workloads []string) []KeyInsight {
	var insights []KeyInsight

	type agentStats struct {
		totalOps    float64
		totalErrors int64
		totalRetries int64
		count       int
	}

	byAgentWorkload := map[string]*agentStats{}
	for _, m := range metrics {
		if m.OpsPerSec <= 0 {
			continue
		}
		key := m.Language + "/" + m.Mode + "/" + m.Workload
		s, ok := byAgentWorkload[key]
		if !ok {
			s = &agentStats{}
			byAgentWorkload[key] = s
		}
		s.totalOps += m.OpsPerSec
		s.totalErrors = m.Errors
		s.totalRetries = m.Retries
		s.count++
	}

	for _, lang := range []string{"java", "go", "python", "dotnet"} {
		libKey := lang + "/library/retry_storm"
		vanKey := lang + "/vanilla/retry_storm"
		libS, libOk := byAgentWorkload[libKey]
		vanS, vanOk := byAgentWorkload[vanKey]
		if libOk && vanOk && (vanS.totalErrors > 0 || libS.totalErrors > 0) {
			if vanS.totalErrors > libS.totalErrors*2 {
				ratio := float64(vanS.totalErrors) / math.Max(float64(libS.totalErrors), 1)
				insights = append(insights, KeyInsight{
					Icon:        "⚡",
					Title:       lang + ": Vanilla retry failures " + fmt.Sprintf("%.0fx", ratio) + " higher",
					Description: fmt.Sprintf("Under contention, vanilla %s accumulated %d errors vs library's %d. The library's retry strategy (exponential backoff + jitter) dramatically reduces failed transactions.", lang, vanS.totalErrors, libS.totalErrors),
					ColorHex:    "ff6b6b",
					ColorRGB:    "255,107,107",
				})
			} else if libS.totalErrors > vanS.totalErrors*2 {
				insights = append(insights, KeyInsight{
					Icon:        "⚠️",
					Title:       lang + ": Library has more errors than vanilla",
					Description: fmt.Sprintf("Library %s had %d errors vs vanilla's %d. This may indicate the retry strategy needs tuning — too aggressive backoff or insufficient max retries.", lang, libS.totalErrors, vanS.totalErrors),
					ColorHex:    "ffd93d",
					ColorRGB:    "255,217,61",
				})
			}
		}
	}

	for _, lang := range []string{"java", "go", "python", "dotnet"} {
		libKey := lang + "/library/crud"
		vanKey := lang + "/vanilla/crud"
		libS, libOk := byAgentWorkload[libKey]
		vanS, vanOk := byAgentWorkload[vanKey]
		if libOk && vanOk && libS.count > 0 && vanS.count > 0 {
			libAvg := libS.totalOps / float64(libS.count)
			vanAvg := vanS.totalOps / float64(vanS.count)
			if vanAvg > 0 {
				overhead := ((vanAvg - libAvg) / vanAvg) * 100
				if overhead > 15 {
					insights = append(insights, KeyInsight{
						Icon:        "📊",
						Title:       fmt.Sprintf("%s: Library has %.0f%% CRUD overhead", lang, overhead),
						Description: fmt.Sprintf("The %s library averages %.0f ops/s vs vanilla's %.0f ops/s. Consider optimizing the middleware pipeline — bypass retry logic for simple reads.", lang, libAvg, vanAvg),
						ColorHex:    "ffd93d",
						ColorRGB:    "255,217,61",
					})
				} else if overhead < -5 {
					insights = append(insights, KeyInsight{
						Icon:        "✅",
						Title:       fmt.Sprintf("%s: Library is %.0f%% faster on CRUD", lang, -overhead),
						Description: fmt.Sprintf("The %s library's connection pool optimization outweighs middleware cost: %.0f ops/s vs vanilla's %.0f ops/s.", lang, libAvg, vanAvg),
						ColorHex:    "4ecdc4",
						ColorRGB:    "78,205,196",
					})
				}
			}
		}
	}

	if len(insights) == 0 {
		insights = append(insights, KeyInsight{
			Icon:        "ℹ️",
			Title:       "Run more workloads for deeper insights",
			Description: "Key insights are generated from retry storm (error comparison) and CRUD (overhead measurement). Run both workloads to see library vs vanilla analysis.",
			ColorHex:    "888888",
			ColorRGB:    "136,136,136",
		})
	}

	return insights
}

func detectIterations(metrics []protocol.MetricSnapshot) int {
	iterIDs := map[string]bool{}
	for _, m := range metrics {
		iterIDs[m.RunID] = true
	}
	if len(iterIDs) <= 1 {
		return 1
	}
	return len(iterIDs)
}

func computeIterationStats(metrics []protocol.MetricSnapshot, key agentKey) (mean, stddev, ci95Lower, ci95Upper float64, iterCount int) {
	iterOps := map[string]*struct {
		sum   float64
		count int
	}{}
	for _, m := range metrics {
		if m.Language != key.Language || m.Mode != key.Mode {
			continue
		}
		acc, ok := iterOps[m.RunID]
		if !ok {
			acc = &struct {
				sum   float64
				count int
			}{}
			iterOps[m.RunID] = acc
		}
		acc.sum += m.OpsPerSec
		acc.count++
	}

	iterCount = len(iterOps)
	if iterCount <= 1 {
		return 0, 0, 0, 0, iterCount
	}

	iterAvgs := make([]float64, 0, iterCount)
	for _, acc := range iterOps {
		if acc.count > 0 {
			iterAvgs = append(iterAvgs, acc.sum/float64(acc.count))
		}
	}

	n := float64(len(iterAvgs))
	var sum float64
	for _, v := range iterAvgs {
		sum += v
	}
	mean = sum / n

	var varianceSum float64
	for _, v := range iterAvgs {
		diff := v - mean
		varianceSum += diff * diff
	}
	stddev = math.Sqrt(varianceSum / (n - 1))

	stderr := stddev / math.Sqrt(n)
	ci95Lower = mean - 1.96*stderr
	ci95Upper = mean + 1.96*stderr

	return mean, stddev, ci95Lower, ci95Upper, iterCount
}

func buildAgentSummaries(metrics []protocol.MetricSnapshot) []AgentSummary {
	type accumulator struct {
		opsSum   float64
		p50Sum   float64
		p95Sum   float64
		p99Sum   float64
		errors   int64
		retries  int64
		count    int
	}

	agents := map[agentKey]*accumulator{}
	agentOrder := []agentKey{}

	langOrder := map[string]int{"java": 0, "go": 1, "python": 2, "dotnet": 3}
	modeOrder := map[string]int{"library": 0, "vanilla": 1}

	for _, m := range metrics {
		k := agentKey{Language: m.Language, Mode: m.Mode}
		acc, exists := agents[k]
		if !exists {
			acc = &accumulator{}
			agents[k] = acc
			agentOrder = append(agentOrder, k)
		}
		acc.opsSum += m.OpsPerSec
		acc.p50Sum += m.LatencyP50Us
		acc.p95Sum += m.LatencyP95Us
		acc.p99Sum += m.LatencyP99Us
		acc.errors += m.Errors
		acc.retries += m.Retries
		acc.count++
	}

	sort.Slice(agentOrder, func(i, j int) bool {
		li, lj := langOrder[agentOrder[i].Language], langOrder[agentOrder[j].Language]
		if li != lj {
			return li < lj
		}
		return modeOrder[agentOrder[i].Mode] < modeOrder[agentOrder[j].Mode]
	})

	numIterations := detectIterations(metrics)

	summaries := make([]AgentSummary, 0, len(agentOrder))
	for _, k := range agentOrder {
		acc := agents[k]
		n := float64(acc.count)
		if n == 0 {
			n = 1
		}
		aKey := k.Language + "/" + k.Mode
		clr := agentColorMap[aKey]
		if clr == "" { clr = langColorMap[k.Language] }
		if clr == "" { clr = "#888" }

		summary := AgentSummary{
			Language:     k.Language,
			Mode:         k.Mode,
			Color:        clr,
			AvgOpsPerSec: acc.opsSum / n,
			P50Display:   formatLatency(acc.p50Sum / n),
			P95Display:   formatLatency(acc.p95Sum / n),
			P99Display:   formatLatency(acc.p99Sum / n),
			TotalErrors:  acc.errors,
			TotalRetries: acc.retries,
		}

		if numIterations > 1 {
			mean, stddev, ci95Lower, ci95Upper, iterCount := computeIterationStats(metrics, k)
			if iterCount > 1 {
				summary.AvgOpsPerSec = mean
				summary.StdDev = stddev
				summary.CI95Lower = ci95Lower
				summary.CI95Upper = ci95Upper
				summary.Iterations = iterCount
			}
		}

		summaries = append(summaries, summary)
	}
	return summaries
}

func buildWorkloadSections(workloads []string, metrics []protocol.MetricSnapshot) []WorkloadSection {
	sections := make([]WorkloadSection, 0, len(workloads))

	for _, wl := range workloads {
		var wlMetrics []protocol.MetricSnapshot
		for _, m := range metrics {
			if m.Workload == wl {
				wlMetrics = append(wlMetrics, m)
			}
		}

		section := WorkloadSection{Name: wl}
		switch protocol.WorkloadType(wl) {
		case protocol.WorkloadCRUD:
			section.Description = "Basic INSERT, SELECT, UPDATE, DELETE operations on a single table."
			section.WhatWeMeasure = "Overhead cost of the library middleware pipeline on simple operations."
			section.LibraryAdvantage = "Connection pool optimization, error classification. Expect 2-10% overhead."
			section.WhatToExpect = "Vanilla should be slightly faster. This measures the cost, not the benefit."
		case protocol.WorkloadRetryStorm:
			section.Description = "High-contention writes on 10 hot rows with 32 concurrent workers forcing serialization conflicts."
			section.WhatWeMeasure = "Retry handling under contention — the core value proposition of the libraries."
			section.LibraryAdvantage = "Exponential backoff + jitter prevents thundering herd. 3-5x fewer failed transactions."
			section.WhatToExpect = "Library agents should have dramatically fewer errors and higher retry success rates."
		case protocol.WorkloadMixedRW:
			section.Description = "80% reads / 20% writes with JOINs across multiple tables."
			section.WhatWeMeasure = "Overall middleware overhead under realistic production-like conditions."
			section.LibraryAdvantage = "Transaction management, read/write routing. Moderate overhead expected."
			section.WhatToExpect = "Similar to CRUD but with more complex queries. Library overhead should be minimal on reads."
		case protocol.WorkloadFollowerReads:
			section.Description = "Read queries routed to nearest replica via AS OF SYSTEM TIME follower_read_timestamp()."
			section.WhatWeMeasure = "Read latency reduction by routing to followers. Requires 3+ node cluster."
			section.LibraryAdvantage = "Automatic follower read SQL injection. 30-60% lower read latency on multi-node."
			section.WhatToExpect = "On single-node: no difference. On 3+ nodes: library reads should be significantly faster."
		case protocol.WorkloadCDC:
			section.Description = "Continuous writes with CockroachDB changefeed consumer measuring event lag."
			section.WhatWeMeasure = "CDC event processing throughput and end-to-end event lag."
			section.LibraryAdvantage = "Typed event handlers, structured webhook parsing. Cleaner code, similar performance."
			section.WhatToExpect = "Similar throughput. Library value here is developer experience, not raw performance."
		case protocol.WorkloadVectorSearch:
			section.Description = "INSERT 1536-dim vectors and cosine similarity searches on bench_documents."
			section.WhatWeMeasure = "Vector indexing speed and search latency using CockroachDB native VECTOR type."
			section.LibraryAdvantage = "VectorSearchTemplate provides clean API vs raw SQL. Less than 1ms overhead."
			section.WhatToExpect = "Nearly identical performance. Library abstracts SQL but adds negligible overhead."
		}

		agentTimeseries := map[agentKey][]protocol.MetricSnapshot{}
		agentKeys := []agentKey{}
		for _, m := range wlMetrics {
			k := agentKey{Language: m.Language, Mode: m.Mode}
			if _, exists := agentTimeseries[k]; !exists {
				agentKeys = append(agentKeys, k)
			}
			agentTimeseries[k] = append(agentTimeseries[k], m)
		}

		sortAgentKeys(agentKeys)

		timeLabels := buildTimeLabels(wlMetrics)
		section.TimeLabelsJSON = toJS(timeLabels)

		for _, k := range agentKeys {
			series := agentTimeseries[k]
			opsData := make([]float64, len(series))
			for i, m := range series {
				opsData[i] = math.Round(m.OpsPerSec*10) / 10
			}
			agentKey := k.Language + "/" + k.Mode
			color := agentColorMap[agentKey]
			if color == "" {
				color = langColorMap[k.Language]
			}
			if color == "" {
				color = "#888"
			}
			section.OpsDatasets = append(section.OpsDatasets, OpsDataset{
				Label:       k.Language + " " + k.Mode,
				DataJSON:    toJS(opsData),
				Color:       color,
				BorderWidth: 2,
			})
		}

		latLabels := make([]string, len(agentKeys))
		latColors := make([]string, len(agentKeys))
		p50Data := make([]float64, len(agentKeys))
		p95Data := make([]float64, len(agentKeys))
		p99Data := make([]float64, len(agentKeys))

		rows := make([]WorkloadAgentRow, 0, len(agentKeys))
		for i, k := range agentKeys {
			series := agentTimeseries[k]
			latLabels[i] = k.Language + " " + k.Mode
			aKey := k.Language + "/" + k.Mode
			latColors[i] = agentColorMap[aKey]
			if latColors[i] == "" {
				latColors[i] = langColorMap[k.Language]
			}
			if latColors[i] == "" {
				latColors[i] = "#888"
			}

			var opsSum, p50Sum, p95Sum, p99Sum float64
			var errors, retries int64
			errMap := map[string]*protocol.ErrorSample{}
			for _, m := range series {
				opsSum += m.OpsPerSec
				p50Sum += m.LatencyP50Us
				p95Sum += m.LatencyP95Us
				p99Sum += m.LatencyP99Us
				errors += m.Errors
				retries += m.Retries
				for _, es := range m.ErrorSamples {
					if existing, ok := errMap[es.SQLState]; ok {
						if es.Count > existing.Count {
							existing.Count = es.Count
							existing.Message = es.Message
						}
					} else {
						clone := es
						errMap[es.SQLState] = &clone
					}
				}
			}
			n := float64(len(series))
			if n == 0 {
				n = 1
			}

			p50Data[i] = p50Sum / n / 1000
			p95Data[i] = p95Sum / n / 1000
			p99Data[i] = p99Sum / n / 1000

			var errSamples []protocol.ErrorSample
			for _, es := range errMap {
				errSamples = append(errSamples, *es)
			}
			sort.Slice(errSamples, func(a, b int) bool {
				return errSamples[a].Count > errSamples[b].Count
			})

			rowKey := k.Language + "/" + k.Mode
			rowClr := agentColorMap[rowKey]
			if rowClr == "" { rowClr = langColorMap[k.Language] }
			if rowClr == "" { rowClr = "#888" }
			row := WorkloadAgentRow{
				Language:     k.Language,
				Mode:         k.Mode,
				Color:        rowClr,
				AvgOpsPerSec: opsSum / n,
				P50Display:   formatLatency(p50Sum / n),
				P95Display:   formatLatency(p95Sum / n),
				P99Display:   formatLatency(p99Sum / n),
				TotalErrors:  errors,
				TotalRetries: retries,
				ErrorSamples: errSamples,
			}

			wlIterations := detectIterations(wlMetrics)
			if wlIterations > 1 {
				mean, stddev, ci95Lower, ci95Upper, iterCount := computeIterationStats(wlMetrics, k)
				if iterCount > 1 {
					row.AvgOpsPerSec = mean
					row.StdDev = stddev
					row.CI95Lower = ci95Lower
					row.CI95Upper = ci95Upper
					row.Iterations = iterCount
				}
			}

			rows = append(rows, row)
		}

		section.LatencyLabelsJSON = toJS(latLabels)
		section.LatencyColorsJSON = toJS(latColors)
		section.P50DataJSON = toJS(roundSlice(p50Data, 2))
		section.P95DataJSON = toJS(roundSlice(p95Data, 2))
		section.P99DataJSON = toJS(roundSlice(p99Data, 2))
		section.AgentRows = rows

		sections = append(sections, section)
	}

	return sections
}

func buildChaosTimeline(events []protocol.ChaosEvent, runStart time.Time) []ChaosTimelineEntry {
	entries := make([]ChaosTimelineEntry, 0, len(events))
	for _, e := range events {
		offset := e.Timestamp.Sub(runStart)
		entries = append(entries, ChaosTimelineEntry{
			TimeDisplay: fmt.Sprintf("+%ds", int(offset.Seconds())),
			Scenario:    string(e.Scenario),
			Action:      e.Action,
			Target:      e.Target,
		})
	}
	return entries
}

func buildComparisonData(workloads []string, metrics []protocol.MetricSnapshot) ([]string, string) {
	type avgOps struct {
		sum   float64
		count int
	}

	langs := []string{"java", "go", "python", "dotnet"}

	perWorkloadLangMode := map[string]map[string]map[string]*avgOps{}
	for _, wl := range workloads {
		perWorkloadLangMode[wl] = map[string]map[string]*avgOps{}
		for _, lang := range langs {
			perWorkloadLangMode[wl][lang] = map[string]*avgOps{
				"library": {},
				"vanilla": {},
			}
		}
	}

	for _, m := range metrics {
		wlData, ok := perWorkloadLangMode[m.Workload]
		if !ok {
			continue
		}
		langData, ok := wlData[m.Language]
		if !ok {
			continue
		}
		modeData, ok := langData[m.Mode]
		if !ok {
			continue
		}
		modeData.sum += m.OpsPerSec
		modeData.count++
	}

	type dsEntry struct {
		Label           string   `json:"label"`
		Data            []float64 `json:"data"`
		BackgroundColor string   `json:"backgroundColor"`
	}

	datasets := make([]dsEntry, 0, len(langs))
	for _, lang := range langs {
		ds := dsEntry{
			Label:           lang,
			Data:            make([]float64, len(workloads)),
			BackgroundColor: langColorMap[lang],
		}
		for j, wl := range workloads {
			lib := perWorkloadLangMode[wl][lang]["library"]
			van := perWorkloadLangMode[wl][lang]["vanilla"]
			if lib.count > 0 && van.count > 0 {
				libAvg := lib.sum / float64(lib.count)
				vanAvg := van.sum / float64(van.count)
				if vanAvg > 0 {
					ds.Data[j] = math.Round((vanAvg-libAvg)/vanAvg*1000) / 10
				}
			}
		}
		datasets = append(datasets, ds)
	}

	dsJSON, _ := json.Marshal(datasets)
	return workloads, string(dsJSON)
}

func buildTimeLabels(metrics []protocol.MetricSnapshot) []string {
	labels := make([]string, len(metrics))
	if len(metrics) == 0 {
		return labels
	}
	baseTS := metrics[0].Timestamp
	seen := map[int64]bool{}
	idx := 0
	for _, m := range metrics {
		if seen[m.Timestamp] {
			continue
		}
		seen[m.Timestamp] = true
		offset := m.Timestamp - baseTS
		if idx < len(labels) {
			labels[idx] = fmt.Sprintf("%ds", offset)
			idx++
		}
	}
	return labels[:idx]
}

func sortAgentKeys(keys []agentKey) {
	langOrder := map[string]int{"java": 0, "go": 1, "python": 2, "dotnet": 3}
	modeOrder := map[string]int{"library": 0, "vanilla": 1}
	sort.Slice(keys, func(i, j int) bool {
		li, lj := langOrder[keys[i].Language], langOrder[keys[j].Language]
		if li != lj {
			return li < lj
		}
		return modeOrder[keys[i].Mode] < modeOrder[keys[j].Mode]
	})
}

func formatLatency(us float64) string {
	if us >= 1000000 {
		return fmt.Sprintf("%.1fs", us/1000000)
	}
	if us >= 1000 {
		return fmt.Sprintf("%.2fms", us/1000)
	}
	return fmt.Sprintf("%.0fus", us)
}

func roundSlice(vals []float64, decimals int) []float64 {
	factor := math.Pow(10, float64(decimals))
	out := make([]float64, len(vals))
	for i, v := range vals {
		out[i] = math.Round(v*factor) / factor
	}
	return out
}

func toJS(v interface{}) template.JS {
	b, _ := json.Marshal(v)
	return template.JS(b)
}
