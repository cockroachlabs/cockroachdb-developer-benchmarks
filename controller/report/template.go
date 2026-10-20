package report

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>CockroachDB Benchmark Report — {{.Run.ID}}</title>
<script src="https://cdn.jsdelivr.net/npm/chart.js@4.4.8/dist/chart.umd.min.js" integrity="sha384-T/4KgSWuZEPozpPz7rnnp/5lDSnpY1VPJCojf1S81uTHS1E38qgLfMgVsAeRCWc4" crossorigin="anonymous"></script>
<style>
*{margin:0;padding:0;box-sizing:border-box}
:root{
  --bg:#0a0a0a;--surface:#141414;--surface2:#1e1e1e;--border:#2a2a2a;
  --text:#e0e0e0;--text-dim:#888;--accent:#6366f1;
  --java:#4ecdc4;--go:#ff6b6b;--python:#ffd93d;--dotnet:#c084fc;
}
body{background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,monospace;line-height:1.6;padding:20px}
.container{max-width:1400px;margin:0 auto}
h1{font-size:1.6rem;margin-bottom:4px}
h2{font-size:1.2rem;margin:32px 0 16px;border-bottom:1px solid var(--border);padding-bottom:8px}
h3{font-size:1rem;margin:20px 0 10px;color:var(--text)}

.header{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:24px;margin-bottom:24px}
.header-meta{display:flex;flex-wrap:wrap;gap:24px;margin-top:12px}
.header-meta .item{display:flex;flex-direction:column}
.header-meta .label{font-size:0.75rem;color:var(--text-dim);text-transform:uppercase;letter-spacing:0.05em}
.header-meta .value{font-size:1rem;font-weight:600}

.tabs{display:flex;flex-wrap:wrap;gap:4px;margin-bottom:16px}
.tab{padding:8px 16px;background:var(--surface);border:1px solid var(--border);border-radius:6px 6px 0 0;cursor:pointer;font-size:0.85rem;color:#aaa;transition:all 0.2s}
.tab:hover{background:var(--surface2);color:#fff}
.tab.active{background:var(--surface2);color:#fff;border-bottom-color:var(--surface2);font-weight:600}
.tab-content{display:none;background:var(--surface2);border:1px solid var(--border);border-radius:0 8px 8px 8px;padding:24px}
.tab-content.active{display:block}

table{width:100%;border-collapse:collapse;font-size:0.85rem;margin:12px 0}
th{text-align:left;padding:10px 12px;background:var(--surface);border-bottom:2px solid var(--border);font-weight:600;font-size:0.75rem;text-transform:uppercase;letter-spacing:0.05em;color:#ccc}
td{padding:8px 12px;border-bottom:1px solid var(--border);color:var(--text)}
tr:hover td{background:rgba(255,255,255,0.02)}
.lang-badge{display:inline-block;padding:2px 8px;border-radius:4px;font-size:0.75rem;font-weight:700;color:#fff}
.lang-java{background:var(--java);color:#0a0a0a}
.lang-java-native{background:#2ecc71;color:#0a0a0a}
.lang-go{background:var(--go);color:#fff}
.lang-python{background:var(--python);color:#0a0a0a}
.lang-dotnet{background:var(--dotnet);color:#fff}
.mode-library td{font-weight:600;color:var(--text)}
.mode-vanilla td{font-weight:400;color:#bbb}

.chart-container{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:16px;margin:12px 0}
.chart-row{display:grid;grid-template-columns:1fr 1fr;gap:16px}
@media(max-width:900px){.chart-row{grid-template-columns:1fr}}

.timeline{background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:16px;margin:12px 0}
.timeline-event{display:flex;align-items:center;gap:12px;padding:8px 0;border-bottom:1px solid var(--border)}
.timeline-event:last-child{border-bottom:none}
.timeline-dot{width:10px;height:10px;border-radius:50%;background:var(--accent);flex-shrink:0}
.timeline-time{font-size:0.75rem;color:var(--text-dim);min-width:80px}
.timeline-label{font-size:0.85rem}
.timeline-target{font-size:0.75rem;color:var(--text-dim)}

.footer{margin-top:40px;padding:16px;text-align:center;font-size:0.75rem;color:var(--text-dim);border-top:1px solid var(--border)}

@media print{
  body{background:#fff;color:#000;padding:10px}
  .header,.tab-content,.chart-container,.timeline{background:#fff;border-color:#ddd}
  th{background:#f5f5f5}
  .tabs{display:none}
  .tab-content{display:block!important;margin-bottom:20px;page-break-inside:avoid}
  .footer a{color:#000}
}
</style>
</head>
<body>
<div class="container">

<div class="header">
  <h1>CockroachDB Developer Benchmark Report</h1>
  <div class="header-meta">
    <div class="item"><span class="label">Run ID</span><span class="value">{{.Run.ID}}</span></div>
    <div class="item"><span class="label">Timestamp</span><span class="value">{{.RunTimestamp}}</span></div>
    <div class="item"><span class="label">Topology</span><span class="value">{{.Run.Topology}}</span></div>
    <div class="item"><span class="label">Duration</span><span class="value">{{.Run.Duration}}s</span></div>
    <div class="item"><span class="label">Workloads</span><span class="value">{{.WorkloadCount}}</span></div>
    <div class="item"><span class="label">Agents</span><span class="value">{{.AgentCount}}</span></div>
    {{if gt .Run.Iterations 1}}<div class="item"><span class="label">Iterations</span><span class="value">{{.Run.Iterations}}</span></div>{{end}}
  </div>
</div>

{{if .Executive.FastestAgent}}
<div style="background:linear-gradient(135deg,rgba(99,102,241,0.08),rgba(78,205,196,0.08));border:1px solid rgba(99,102,241,0.3);border-radius:8px;padding:24px;margin-bottom:24px">
  <h2 style="margin:0 0 16px;border:none;padding:0;color:var(--accent)">Executive Summary</h2>
  <div style="display:grid;grid-template-columns:1fr 1fr 1fr;gap:16px;margin-bottom:16px">
    <div style="background:var(--surface);border-radius:6px;padding:14px;text-align:center">
      <div style="font-size:11px;color:var(--text-dim);text-transform:uppercase;margin-bottom:4px">Fastest Agent</div>
      <div style="font-size:1.1rem;font-weight:700;color:#4ecdc4">{{.Executive.FastestAgent}}</div>
      <div style="font-size:13px;color:var(--text-dim)">{{printf "%.0f" .Executive.FastestOps}} ops/sec</div>
    </div>
    <div style="background:var(--surface);border-radius:6px;padding:14px;text-align:center">
      <div style="font-size:11px;color:var(--text-dim);text-transform:uppercase;margin-bottom:4px">Best Library</div>
      <div style="font-size:1.1rem;font-weight:700;color:#c084fc">{{.Executive.BestLibrary}}</div>
      <div style="font-size:13px;color:var(--text-dim)">{{printf "%.0f" .Executive.BestLibraryOps}} ops/sec</div>
    </div>
    <div style="background:var(--surface);border-radius:6px;padding:14px;text-align:center">
      <div style="font-size:11px;color:var(--text-dim);text-transform:uppercase;margin-bottom:4px">Verdict</div>
      <div style="font-size:12px;line-height:1.5;color:var(--text)">{{.Executive.OverallVerdict}}</div>
    </div>
  </div>
  {{if .Executive.TopFindings}}
  <div style="font-size:12px;line-height:1.8;color:var(--text)">
    {{range .Executive.TopFindings}}<div style="padding-left:12px;border-left:2px solid var(--accent);margin-bottom:4px">{{.}}</div>{{end}}
  </div>
  {{end}}
</div>
{{end}}

<h2>Summary</h2>
<table>
  <thead>
    <tr>
      <th>Language</th><th>Mode</th><th>Avg Ops/sec <span style="color:#4ecdc4;font-size:10px">↑</span></th>
      {{if gt .Run.Iterations 1}}<th>CI 95%</th>{{end}}
      <th>P50 Latency <span style="color:#ff6b6b;font-size:10px">↓</span></th><th>P95 Latency <span style="color:#ff6b6b;font-size:10px">↓</span></th><th>P99 Latency <span style="color:#ff6b6b;font-size:10px">↓</span></th>
      <th>Errors <span style="color:#ff6b6b;font-size:10px">↓</span></th><th>Retries <span style="color:#ff6b6b;font-size:10px">↓</span></th>
    </tr>
  </thead>
  <tbody>
    {{range .AgentSummaries}}
    <tr>
      <td><span style="display:inline-block;padding:2px 10px;border-radius:4px;font-size:0.75rem;font-weight:700;background:{{.Color}};color:#fff">{{.Language}}</span></td>
      <td style="color:#ccc">{{.Mode}}</td>
      <td>{{if gt .Iterations 1}}{{printf "%.1f" .AvgOpsPerSec}} <span style="color:#888;font-size:11px">± {{printf "%.1f" .StdDev}}</span>{{else}}{{printf "%.1f" .AvgOpsPerSec}}{{end}}</td>
      {{if gt $.Run.Iterations 1}}<td style="font-size:12px;color:#aaa">{{if gt .Iterations 1}}{{printf "%.0f" .CI95Lower}} – {{printf "%.0f" .CI95Upper}}{{else}}—{{end}}</td>{{end}}
      <td>{{.P50Display}}</td>
      <td>{{.P95Display}}</td>
      <td>{{.P99Display}}</td>
      <td>{{.TotalErrors}}</td>
      <td>{{.TotalRetries}}</td>
    </tr>
    {{end}}
  </tbody>
</table>

<div style="margin:24px 0;padding:20px;background:linear-gradient(135deg,rgba(78,205,196,0.05),rgba(192,132,252,0.05));border:1px solid rgba(78,205,196,0.2);border-radius:8px">
  <h3 style="margin-top:0;color:#4ecdc4">Understanding Library Value</h3>
  <div style="display:grid;grid-template-columns:1fr 1fr;gap:16px;font-size:13px;line-height:1.7">
    <div>
      <p><strong style="color:#ff6b6b">CRUD / Mixed R/W</strong> — These workloads measure the <em>overhead cost</em> of the library middleware pipeline. Libraries add connection pool optimization, error classification, and retry infrastructure. Expect 2-10% overhead on simple operations. This is the price of safety — not where libraries prove their value.</p>
      <p><strong style="color:#4ecdc4">Retry Storm</strong> — This is where libraries shine. Under high contention (SQLSTATE 40001), vanilla code with hand-rolled retry loops creates thundering herd effects. Libraries use exponential backoff + jitter to spread retries intelligently. Python's dialect uses CockroachDB's server-side savepoint restart protocol. Expect 3-5x fewer failed transactions.</p>
    </div>
    <div>
      <p><strong style="color:#c084fc">Follower Reads</strong> — On multi-node clusters, libraries automatically route reads to the nearest replica via AS OF SYSTEM TIME follower_read_timestamp(). No SQL changes needed. Expect 30-60% lower read latency. Single-node benchmarks cannot demonstrate this advantage.</p>
      <p><strong style="color:#ffd93d">Chaos Engineering</strong> — Under node failures and network partitions, libraries with circuit breakers and automatic retry recover gracefully. Vanilla code hard-fails until manual intervention. This is where the library investment pays for itself in production reliability.</p>
    </div>
  </div>
  <p style="margin-bottom:0;font-size:12px;color:#888"><em>The narrative: "Yes, the library costs a small amount on simple operations. But it pays for itself 10x on the scenarios that matter — contention, multi-region, and failure recovery."</em></p>
</div>

<h2>Per-Workload Detail</h2>
<div class="tabs" id="workload-tabs">
  {{range $i, $w := .Workloads}}
  <div class="tab{{if eq $i 0}} active{{end}}" onclick="switchTab({{$i}})">{{$w}}</div>
  {{end}}
</div>

{{range $i, $w := .WorkloadSections}}
<div class="tab-content{{if eq $i 0}} active{{end}}" id="workload-{{$i}}">
  <h3>{{$w.Name}}</h3>
  {{if $w.Description}}
  <div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;padding:12px;margin-bottom:12px;background:rgba(255,255,255,0.02);border:1px solid var(--border);border-radius:6px;font-size:12px;line-height:1.6">
    <div><span style="color:#888;font-size:10px;text-transform:uppercase;font-weight:600">What this benchmark does</span><br/>{{$w.Description}}</div>
    <div><span style="color:#888;font-size:10px;text-transform:uppercase;font-weight:600">What we measure</span><br/>{{$w.WhatWeMeasure}}</div>
    <div><span style="color:#4ecdc4;font-size:10px;text-transform:uppercase;font-weight:600">Library advantage</span><br/>{{$w.LibraryAdvantage}}</div>
    <div><span style="color:#ffd93d;font-size:10px;text-transform:uppercase;font-weight:600">What to expect</span><br/>{{$w.WhatToExpect}}</div>
  </div>
  {{end}}
  <div class="chart-row">
    <div class="chart-container">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px">
        <span style="font-weight:600;font-size:13px">Throughput (ops/s)</span>
        <span style="font-size:11px;padding:2px 8px;border-radius:4px;background:rgba(78,205,196,0.15);color:#4ecdc4">↑ Higher is better</span>
      </div>
      <canvas id="ops-chart-{{$i}}" height="200"></canvas>
    </div>
    <div class="chart-container">
      <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px">
        <span style="font-weight:600;font-size:13px">Latency (ms)</span>
        <span style="font-size:11px;padding:2px 8px;border-radius:4px;background:rgba(255,107,107,0.15);color:#ff6b6b">↓ Lower is better</span>
      </div>
      <canvas id="latency-chart-{{$i}}" height="200"></canvas>
    </div>
  </div>
  <table>
    <thead>
      <tr>
        <th>Language</th><th>Mode</th><th>Avg Ops/sec <span style="color:#4ecdc4;font-size:10px">↑</span></th>
        {{if gt $.Run.Iterations 1}}<th>CI 95%</th>{{end}}
        <th>P50 <span style="color:#ff6b6b;font-size:10px">↓</span></th><th>P95 <span style="color:#ff6b6b;font-size:10px">↓</span></th><th>P99 <span style="color:#ff6b6b;font-size:10px">↓</span></th>
        <th>Errors <span style="color:#ff6b6b;font-size:10px">↓</span></th><th>Retries <span style="color:#ff6b6b;font-size:10px">↓</span></th>
      </tr>
    </thead>
    <tbody>
      {{range $w.AgentRows}}
      <tr>
        <td><span style="display:inline-block;padding:2px 10px;border-radius:4px;font-size:0.75rem;font-weight:700;background:{{.Color}};color:#fff">{{.Language}}</span></td>
        <td style="color:#ccc">{{.Mode}}</td>
        <td>{{if gt .Iterations 1}}{{printf "%.1f" .AvgOpsPerSec}} <span style="color:#888;font-size:11px">± {{printf "%.1f" .StdDev}}</span>{{else}}{{printf "%.1f" .AvgOpsPerSec}}{{end}}</td>
        {{if gt $.Run.Iterations 1}}<td style="font-size:12px;color:#aaa">{{if gt .Iterations 1}}{{printf "%.0f" .CI95Lower}} – {{printf "%.0f" .CI95Upper}}{{else}}—{{end}}</td>{{end}}
        <td>{{.P50Display}}</td>
        <td>{{.P95Display}}</td>
        <td>{{.P99Display}}</td>
        <td>{{if .ErrorSamples}}<details><summary style="cursor:pointer;color:#ff6b6b">{{.TotalErrors}}</summary><div style="margin-top:4px;font-size:11px">{{range .ErrorSamples}}<div>SQLSTATE {{.SQLState}}: {{.Message}} (&times;{{.Count}})</div>{{end}}</div></details>{{else}}{{.TotalErrors}}{{end}}</td>
        <td>{{.TotalRetries}}</td>
      </tr>
      {{end}}
    </tbody>
  </table>
</div>
{{end}}

<h2>Library vs Vanilla Comparison</h2>
<div class="chart-container">
  <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:4px">
    <span style="font-weight:600;font-size:13px">Throughput: Library (solid) vs Vanilla (striped) — per language</span>
    <span style="font-size:11px;padding:2px 8px;border-radius:4px;background:rgba(78,205,196,0.15);color:#4ecdc4">↑ Higher is better</span>
  </div>
  <canvas id="comparison-chart" height="300"></canvas>
</div>

{{if .KeyInsights}}
<h2>Key Insights</h2>
<div style="display:grid;grid-template-columns:1fr 1fr;gap:12px;margin-bottom:24px">
  {{range .KeyInsights}}
  <div style="padding:12px;background:rgba({{.ColorRGB}},0.05);border:1px solid rgba({{.ColorRGB}},0.2);border-radius:6px">
    <div style="font-weight:600;font-size:13px;color:#{{.ColorHex}};margin-bottom:4px">{{.Icon}} {{.Title}}</div>
    <div style="font-size:12px;line-height:1.6;color:var(--text-secondary)">{{.Description}}</div>
  </div>
  {{end}}
</div>
{{end}}

{{if .HasRegressions}}
<h2>Regression Alerts</h2>
<div style="display:grid;grid-template-columns:1fr;gap:8px;margin-bottom:24px">
  {{range .RegressionAlerts}}
  <div style="padding:12px 16px;border-radius:6px;display:flex;align-items:center;gap:12px;{{if .IsRegression}}background:rgba(255,107,107,0.08);border:1px solid rgba(255,107,107,0.3){{else}}background:rgba(78,205,196,0.08);border:1px solid rgba(78,205,196,0.3){{end}}">
    <span style="font-size:18px">{{if .IsRegression}}&#x26A0;{{else}}&#x2705;{{end}}</span>
    <div style="flex:1">
      <div style="font-weight:600;font-size:13px;{{if .IsRegression}}color:#ff6b6b{{else}}color:#4ecdc4{{end}}">{{.Agent}} — {{.Metric}}</div>
      <div style="font-size:12px;color:var(--text-dim)">{{printf "%.0f" .PreviousValue}} → {{printf "%.0f" .CurrentValue}} ({{if gt .ChangePct 0.0}}+{{end}}{{printf "%.1f" .ChangePct}}%)</div>
    </div>
  </div>
  {{end}}
</div>
{{end}}

{{if .HasChaos}}
<h2>Chaos Events Timeline</h2>
<div class="timeline">
  {{range .ChaosTimeline}}
  <div class="timeline-event">
    <div class="timeline-dot"></div>
    <div class="timeline-time">{{.TimeDisplay}}</div>
    <div class="timeline-label">{{.Scenario}} — {{.Action}}</div>
    <div class="timeline-target">{{.Target}}</div>
  </div>
  {{end}}
</div>
{{end}}

{{if .ProsCons}}
<h2>Pros vs Cons — Per Library</h2>
<div style="display:grid;grid-template-columns:repeat(auto-fit,minmax(280px,1fr));gap:16px;margin-bottom:24px">
  {{range .ProsCons}}
  <div style="background:var(--surface);border:1px solid var(--border);border-radius:8px;overflow:hidden">
    <div style="padding:10px 14px;font-weight:700;font-size:14px;border-bottom:1px solid var(--border);color:#{{.ColorHex}}">
      {{.Language}}
    </div>
    <div style="display:grid;grid-template-columns:1fr 1fr;min-height:120px">
      <div style="padding:10px 14px;border-right:1px solid var(--border)">
        <div style="font-size:10px;text-transform:uppercase;font-weight:600;color:#4ecdc4;margin-bottom:6px">✓ Pros</div>
        {{range .Pros}}
        <div style="font-size:12px;line-height:1.6;padding:2px 0;color:var(--text-secondary)">• {{.Text}}</div>
        {{end}}
      </div>
      <div style="padding:10px 14px">
        <div style="font-size:10px;text-transform:uppercase;font-weight:600;color:#ff6b6b;margin-bottom:6px">✗ Cons</div>
        {{range .Cons}}
        <div style="font-size:12px;line-height:1.6;padding:2px 0;color:var(--text-secondary)">• {{.Text}}</div>
        {{end}}
        {{if not .Cons}}
        <div style="font-size:12px;color:#555;font-style:italic">No significant cons detected</div>
        {{end}}
      </div>
    </div>
  </div>
  {{end}}
</div>
{{end}}

<div class="footer">
  Report generated {{.GeneratedAt}}<br>
  <a href="/" style="color:var(--accent);text-decoration:none">Back to Dashboard</a>
</div>

</div>

<script>
function switchTab(idx) {
  document.querySelectorAll('.tab').forEach((t,i) => t.classList.toggle('active', i===idx));
  document.querySelectorAll('.tab-content').forEach((c,i) => c.classList.toggle('active', i===idx));
}

const chartDefaults = {
  responsive: true,
  plugins: {legend:{labels:{color:'#ccc',font:{size:11},usePointStyle:true,pointStyle:'circle'}}},
  scales: {
    x: {ticks:{color:'#aaa',font:{size:10}},grid:{color:'#333'}},
    y: {ticks:{color:'#aaa',font:{size:10}},grid:{color:'#333'}}
  }
};

{{range $i, $w := .WorkloadSections}}
(function(){
  const opsCtx = document.getElementById('ops-chart-{{$i}}');
  if (!opsCtx) return;
  new Chart(opsCtx, {
    type: 'line',
    data: {
      labels: {{$w.TimeLabelsJSON}},
      datasets: [
        {{range $w.OpsDatasets}}
        {label:'{{.Label}}',data:{{.DataJSON}},borderColor:'{{.Color}}',backgroundColor:'{{.Color}}22',borderWidth:{{.BorderWidth}},pointRadius:0,tension:0.3},
        {{end}}
      ]
    },
    options: {...chartDefaults}
  });

  const latCtx = document.getElementById('latency-chart-{{$i}}');
  if (!latCtx) return;
  new Chart(latCtx, {
    type: 'bar',
    data: {
      labels: {{$w.LatencyLabelsJSON}},
      datasets: [
        {label:'P50',data:{{$w.P50DataJSON}},backgroundColor:{{$w.LatencyColorsJSON}}},
        {label:'P95',data:{{$w.P95DataJSON}},backgroundColor:{{$w.LatencyColorsJSON}}},
        {label:'P99',data:{{$w.P99DataJSON}},backgroundColor:{{$w.LatencyColorsJSON}}}
      ]
    },
    options: {...chartDefaults}
  });
})();
{{end}}

(function(){
  const ctx = document.getElementById('comparison-chart');
  if (!ctx) return;
  new Chart(ctx, {
    type: 'bar',
    data: {
      labels: {{.ComparisonLabelsJSON}},
      datasets: {{.ComparisonDatasetsJSON}}
    },
    options: {
      ...chartDefaults,
      plugins:{
        ...chartDefaults.plugins,
        title:{display:true,text:'Library vs Vanilla Overhead (% difference in ops/sec, negative = library faster)',color:'#ccc'}
      },
      scales:{
        ...chartDefaults.scales,
        y:{...chartDefaults.scales.y,ticks:{...chartDefaults.scales.y.ticks,callback:function(v){return v+'%'}}}
      }
    }
  });
})();
</script>
</body>
</html>`
