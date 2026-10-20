from agent.metrics import MetricsRecorder
from agent.workloads.base import WorkloadRunner
from agent.workloads.cdc import CDCWorkload
from agent.workloads.crud import CRUDWorkload
from agent.workloads.follower_reads import FollowerReadsWorkload
from agent.workloads.mixed_rw import MixedRWWorkload
from agent.workloads.retry_storm import RetryStormWorkload
from agent.workloads.vector_search import VectorSearchWorkload

_WORKLOADS: dict[str, type[WorkloadRunner]] = {
    "crud": CRUDWorkload,
    "retry_storm": RetryStormWorkload,
    "mixed_rw": MixedRWWorkload,
    "follower_reads": FollowerReadsWorkload,
    "cdc": CDCWorkload,
    "vector_search": VectorSearchWorkload,
}


def get_workload(workload_type: str, mode: str, recorder: MetricsRecorder) -> WorkloadRunner | None:
    cls = _WORKLOADS.get(workload_type)
    if cls is None:
        return None
    return cls(mode=mode, recorder=recorder)
