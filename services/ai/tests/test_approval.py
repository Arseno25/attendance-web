"""Policy fixtures are synthetic attestations, not an accuracy report."""

import hashlib
import json
from dataclasses import asdict

from app.face_engine import EngineConfig


def test_activation_requires_exact_reviewed_artifact_and_matching_models(tmp_path):
    from app.approval import load_approval, runtime_fingerprint

    config = EngineConfig(0.8, 0.1)
    report = {
        "kind": "holdout",
        "model_sha256": "a" * 64,
        "pad_model_sha256": "b" * 64,
        "liveness_threshold": 0.95,
        "detector_sha256": "c" * 64,
        "engine_fingerprint": runtime_fingerprint(),
        "config": asdict(config),
        "attack_scenarios": {"print": 20, "replay": 20},
        "reports": [
            {
                "threshold": 0.8,
                "margin": 0.1,
                "known_probes": 500,
                "unknown_probes": 3000,
                "attack_probes": 40,
                "fpir": 0,
                "known_wrong_identifications": 0,
                "attack_pass_rate": 0,
            }
        ],
        "review": {
            "automatic_attendance_approved": True,
            "holdout_verified": True,
            "liveness_verified": True,
            "stability_verified": True,
            "threshold_margin_verified": True,
            "liveness_threshold": 0.95,
            "camera_to_ui_p95_ms": 1400,
        },
    }
    path = tmp_path / "reviewed.json"

    def check(data=report, digest=None):
        raw = json.dumps(data).encode()
        path.write_bytes(raw)
        return load_approval(
            path,
            hashlib.sha256(raw).hexdigest() if digest is None else digest,
            "a" * 64,
            "b" * 64,
            config,
            0.95,
            "c" * 64,
        )

    approved = check()
    assert approved["automatic_attendance_approved"] is True
    assert approved["report_sha256"] == hashlib.sha256(path.read_bytes()).hexdigest()
    assert check(digest="c" * 64) is None
    changed = dict(
        report,
        detector_sha256="different-detector",
        engine_fingerprint="different-code",
    )
    assert check(changed) is None
    for key, value in [
        ("model_sha256", "c" * 64),
        ("kind", "calibration"),
        ("pad_model_sha256", "c" * 64),
    ]:
        changed = dict(report, **{key: value})
        assert check(changed) is None
    for change in (
        "no_review",
        "truthy_flags",
        "few_unknowns",
        "attack",
        "latency",
        "threshold",
        "missing_replay",
    ):
        changed = json.loads(json.dumps(report))
        if change == "no_review":
            changed["review"] = {}
        if change == "truthy_flags":
            changed["review"]["automatic_attendance_approved"] = "true"
        if change == "few_unknowns":
            changed["reports"][0]["unknown_probes"] = 10
        if change == "attack":
            changed["reports"][0]["attack_pass_rate"] = 0.1
        if change == "latency":
            changed["review"]["camera_to_ui_p95_ms"] = 2500
        if change == "threshold":
            changed["config"]["threshold"] = 0.9
        if change == "missing_replay":
            changed["attack_scenarios"] = {"print": 40}
        assert check(changed) is None, change
    for value in (0.99, None, True, "0.95", float("nan")):
        assert check(dict(report, liveness_threshold=value)) is None
    for key in ("threshold", "margin"):
        changed = json.loads(json.dumps(report))
        changed["reports"][0][key] = 0.99
        assert check(changed) is None
    assert load_approval(None, "", "a" * 64, "b" * 64, config, 0.95, "c" * 64) is None
