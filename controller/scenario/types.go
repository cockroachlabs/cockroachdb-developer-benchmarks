package scenario

type Scenario struct {
	Name         string      `json:"name"`
	Version      string      `json:"version,omitempty"`
	Description  string      `json:"description"`
	Schema       SchemaSetup `json:"schema,omitempty"`
	DurationSecs int         `json:"duration_secs"`
	WarmupSecs   int         `json:"warmup_secs,omitempty"`
	Operations   []Operation `json:"operations"`
	Assertions   []Assertion `json:"assertions,omitempty"`
}

type SchemaSetup struct {
	SetupSQL    []string `json:"setup_sql,omitempty"`
	SeedSQL     []string `json:"seed_sql,omitempty"`
	TeardownSQL []string `json:"teardown_sql,omitempty"`
}

type Operation struct {
	Name        string         `json:"name"`
	Type        OperationType  `json:"type"`
	Concurrency int            `json:"concurrency"`
	Weight      int            `json:"weight"`
	SQL         []string       `json:"sql"`
	Params      map[string]any `json:"params,omitempty"`
	Mode        string         `json:"mode,omitempty"`
	LibraryMode *LibraryConfig `json:"library_mode,omitempty"`
}

type OperationType string

const (
	OpQuery        OperationType = "query"
	OpInsert       OperationType = "insert"
	OpUpdate       OperationType = "update"
	OpDelete       OperationType = "delete"
	OpTransaction  OperationType = "transaction"
	OpFollowerRead OperationType = "follower_read"
)

type LibraryConfig struct {
	UseRetry       bool `json:"use_retry"`
	UseTemplate    bool `json:"use_template"`
	UseFollowerRead bool `json:"use_follower_read"`
}

type Assertion struct {
	Metric      string  `json:"metric"`
	Operator    string  `json:"operator"`
	Value       float64 `json:"value"`
	Description string  `json:"description"`
}
