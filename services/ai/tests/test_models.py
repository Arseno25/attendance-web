import hashlib

import pytest


def test_checksum_rejects_wrong_or_truncated_model(tmp_path):
    from app.models import verify_model

    model = tmp_path / "model.onnx"
    model.write_bytes(b"model")
    spec = {
        "filename": "model.onnx",
        "sha256": hashlib.sha256(b"model").hexdigest(),
        "size": 5,
    }
    assert verify_model(tmp_path, spec) == model
    model.write_bytes(b"wrong")
    with pytest.raises(ValueError):
        verify_model(tmp_path, spec)


def test_evaluation_separates_unknown_false_accept_and_known_misses():
    from app.evaluation import summarize

    report = summarize(
        [
            {"expected": None, "predicted": None, "latency_ms": 100, "attack": False},
            {"expected": None, "predicted": "a", "latency_ms": 200, "attack": False},
            {"expected": "a", "predicted": "a", "latency_ms": 300, "attack": False},
            {"expected": "a", "predicted": "b", "latency_ms": 400, "attack": False},
            {"expected": "a", "predicted": None, "latency_ms": 500, "attack": False},
            {"expected": None, "predicted": "a", "latency_ms": 600, "attack": True},
        ]
    )
    assert report["fpir"] == 0.5
    assert report["fnir"] == 2 / 3
    assert report["known_wrong_identifications"] == 1
    assert report["attack_pass_rate"] == 1
    assert report["p95_latency_ms"] == 600
    assert report["automatic_attendance_approved"] is False
    with pytest.raises(ValueError):
        summarize([])
    with pytest.raises(ValueError):
        summarize(
            [
                {
                    "expected": None,
                    "predicted": None,
                    "latency_ms": float("nan"),
                    "attack": False,
                }
            ]
        )
