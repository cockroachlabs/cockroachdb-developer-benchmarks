package workloads

import (
	"encoding/json"
	"log"
)

type RunnerFactory func(rec *MetricsRecorder) WorkloadRunner

var Registry = map[string]map[bool]RunnerFactory{
	"crud": {
		true:  func(r *MetricsRecorder) WorkloadRunner { return RunLibraryCRUD(r) },
		false: func(r *MetricsRecorder) WorkloadRunner { return RunVanillaCRUD(r) },
	},
	"retry_storm": {
		true:  func(r *MetricsRecorder) WorkloadRunner { return RunLibraryRetryStorm(r) },
		false: func(r *MetricsRecorder) WorkloadRunner { return RunVanillaRetryStorm(r) },
	},
	"mixed_rw": {
		true:  func(r *MetricsRecorder) WorkloadRunner { return RunLibraryMixedRW(r) },
		false: func(r *MetricsRecorder) WorkloadRunner { return RunVanillaMixedRW(r) },
	},
	"follower_reads": {
		true:  func(r *MetricsRecorder) WorkloadRunner { return RunLibraryFollowerReads(r) },
		false: func(r *MetricsRecorder) WorkloadRunner { return RunVanillaFollowerReads(r) },
	},
	"cdc": {
		true:  func(r *MetricsRecorder) WorkloadRunner { return RunLibraryCDC(r) },
		false: func(r *MetricsRecorder) WorkloadRunner { return RunVanillaCDC(r) },
	},
	"vector_search": {
		true:  func(r *MetricsRecorder) WorkloadRunner { return RunLibraryVectorSearch(r) },
		false: func(r *MetricsRecorder) WorkloadRunner { return RunVanillaVectorSearch(r) },
	},
}

func CreateRunner(workloadType string, isLibrary bool, rec *MetricsRecorder) WorkloadRunner {
	modes, ok := Registry[workloadType]
	if !ok {
		return nil
	}
	factory, ok := modes[isLibrary]
	if !ok {
		return nil
	}
	return factory(rec)
}

func CreateScenarioRunner(rec *MetricsRecorder, scenarioData json.RawMessage, mode string) WorkloadRunner {
	runner, err := NewScenarioRunner(rec, scenarioData, mode)
	if err != nil {
		log.Printf("failed to create scenario runner: %v", err)
		return nil
	}
	return runner
}
