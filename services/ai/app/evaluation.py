"""Summarize authorized hold-out decision traces without exporting identities."""

import argparse
import json
import math
from pathlib import Path


def summarize(probes):
    if not probes:
        raise ValueError("No hold-out probes")
    latencies = []
    unknown = known = false_accept = missed = wrong = attacks = passed = 0
    for row in probes:
        expected, predicted, latency, attack = (
            row[k] for k in ("expected", "predicted", "latency_ms", "attack")
        )
        if (
            not isinstance(attack, bool)
            or not isinstance(latency, (int, float))
            or not math.isfinite(latency)
            or latency < 0
        ):
            raise ValueError("Invalid probe")
        if any(
            v is not None and (not isinstance(v, str) or not v)
            for v in (expected, predicted)
        ):
            raise ValueError("Use anonymous nonempty subject IDs or null")
        latencies.append(latency)
        if attack:
            attacks += 1
            passed += predicted is not None
        elif expected is None:
            unknown += 1
            false_accept += predicted is not None
        else:
            known += 1
            missed += predicted != expected
            wrong += predicted is not None and predicted != expected

    def rate(n, d):
        return n / d if d else None

    upper = 1 - 0.05 ** (1 / unknown) if unknown and false_accept == 0 else None
    return {
        "probes": len(probes),
        "known_probes": known,
        "unknown_probes": unknown,
        "attack_probes": attacks,
        "fpir": rate(false_accept, unknown),
        "fnir": rate(missed, known),
        "known_wrong_identifications": wrong,
        "attack_pass_rate": rate(passed, attacks),
        "zero_false_accept_upper95": upper,
        "p95_latency_ms": sorted(latencies)[math.ceil(len(latencies) * 0.95) - 1],
        "automatic_attendance_approved": False,
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("trace", type=Path)
    args = parser.parse_args()
    print(json.dumps(summarize(json.loads(args.trace.read_text())), indent=2))
