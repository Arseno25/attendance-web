"""Load only the exact operator-reviewed camera report; missing evidence stays disabled."""

import hashlib
import hmac
import json
import math
from dataclasses import asdict
from pathlib import Path

GATES = (
    "automatic_attendance_approved",
    "holdout_verified",
    "liveness_verified",
    "stability_verified",
    "threshold_margin_verified",
)


def runtime_fingerprint():
    files = (
        "face_engine.py",
        "pipeline.py",
        "liveness.py",
        "recognition.py",
        "main.py",
        "engine_tools.py",
        "approval.py",
    )
    return hashlib.sha256(
        b"".join(Path(__file__).with_name(name).read_bytes() for name in files)
    ).hexdigest()


def load_approval(
    path, expected_sha, model_sha, pad_sha, config, liveness_threshold, detector_sha
):
    if not path or not expected_sha:
        return None
    try:
        with Path(path).open("rb") as source:
            raw = source.read((1 << 20) + 1)
        digest = hashlib.sha256(raw).hexdigest()
        if len(raw) > 1 << 20 or not hmac.compare_digest(digest, expected_sha):
            return None
        report = json.loads(raw)
        if (
            report.get("detector_sha256") != detector_sha
            or report.get("engine_fingerprint") != runtime_fingerprint()
        ):
            return None
        if (
            report["kind"] != "holdout"
            or report["model_sha256"] != model_sha
            or report["pad_model_sha256"] != pad_sha
            or report["config"] != asdict(config)
        ):
            return None
        evaluated_pad = report["liveness_threshold"]
        if (
            type(evaluated_pad) not in (int, float)
            or not math.isfinite(evaluated_pad)
            or not 0 < evaluated_pad <= 1
            or evaluated_pad != liveness_threshold
        ):
            return None
        review = report["review"]
        if (
            any(review.get(gate) is not True for gate in GATES)
            or review["liveness_threshold"] != liveness_threshold
        ):
            return None
        latency = review["camera_to_ui_p95_ms"]
        if (
            type(latency) not in (int, float)
            or not math.isfinite(latency)
            or not 0 < latency <= 2000
        ):
            return None
        if len(report["reports"]) != 1:
            return None
        metrics = report["reports"][0]
        if (
            metrics["threshold"] != config.threshold
            or metrics["margin"] != config.margin
        ):
            return None
        if any(
            type(metrics[key]) is not int or metrics[key] <= 0
            for key in ("known_probes", "unknown_probes", "attack_probes")
        ):
            return None
        # PRD FPIR target 0.1%; zero false accepts need sufficient independent probes.
        if 1 - 0.05 ** (1 / metrics["unknown_probes"]) > 0.001:
            return None
        for key in ("fpir", "known_wrong_identifications", "attack_pass_rate"):
            if type(metrics[key]) not in (int, float) or metrics[key] != 0:
                return None
        scenarios = report["attack_scenarios"]
        if any(
            type(scenarios.get(key)) is not int or scenarios[key] <= 0
            for key in ("print", "replay")
        ):
            return None
        if (
            any(type(n) is not int or n < 0 for n in scenarios.values())
            or sum(scenarios.values()) != metrics["attack_probes"]
        ):
            return None
        return {
            "status": "reviewed",
            **{gate: True for gate in GATES},
            "model_sha256": model_sha,
            "report_sha256": digest,
        }
    except OSError, ValueError, TypeError, KeyError, IndexError, ArithmeticError:
        return None
