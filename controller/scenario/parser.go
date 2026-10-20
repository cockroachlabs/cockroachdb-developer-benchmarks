package scenario

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var validOpTypes = map[OperationType]bool{
	OpQuery:        true,
	OpInsert:       true,
	OpUpdate:       true,
	OpDelete:       true,
	OpTransaction:  true,
	OpFollowerRead: true,
}

func Parse(data []byte) (*Scenario, error) {
	var s Scenario
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse scenario JSON: %w", err)
	}
	if err := Validate(&s); err != nil {
		return nil, err
	}
	return &s, nil
}

func LoadFile(path string) (*Scenario, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read scenario file %s: %w", path, err)
	}
	return Parse(data)
}

func LoadDir(dir string) ([]*Scenario, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read scenario directory %s: %w", dir, err)
	}

	var scenarios []*Scenario
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		s, err := LoadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		scenarios = append(scenarios, s)
	}
	return scenarios, nil
}

func Validate(s *Scenario) error {
	if s.Name == "" {
		return fmt.Errorf("scenario name is required")
	}
	if s.Description == "" {
		return fmt.Errorf("scenario description is required")
	}
	if s.DurationSecs <= 0 {
		return fmt.Errorf("duration_secs must be positive")
	}
	if len(s.Operations) == 0 {
		return fmt.Errorf("at least one operation is required")
	}
	for i, op := range s.Operations {
		if op.Name == "" {
			return fmt.Errorf("operation %d: name is required", i)
		}
		if !validOpTypes[op.Type] {
			return fmt.Errorf("operation %d (%s): invalid type %q", i, op.Name, op.Type)
		}
		if op.Weight <= 0 {
			return fmt.Errorf("operation %d (%s): weight must be positive", i, op.Name)
		}
		if len(op.SQL) == 0 {
			return fmt.Errorf("operation %d (%s): at least one SQL statement is required", i, op.Name)
		}
	}

	for i, a := range s.Assertions {
		if a.Metric == "" {
			return fmt.Errorf("assertion %d: metric is required", i)
		}
		switch a.Operator {
		case "<", "<=", ">", ">=", "==", "!=":
		default:
			return fmt.Errorf("assertion %d: invalid operator %q", i, a.Operator)
		}
	}

	return nil
}
