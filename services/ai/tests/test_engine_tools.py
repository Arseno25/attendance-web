"""Evaluation protocol checks use an artificial codec, never biometric data."""

import json

import numpy as np
import pytest

from app.engine_tools import evaluate
from app.face_engine import EngineConfig
from app.pipeline import Pipeline

SHA = "a" * 64


class SyntheticPipeline(Pipeline):
    model_sha256 = SHA
    detector_sha256 = "c" * 64

    def __init__(self, _directory):
        pass

    def detect(self, _frame, **_options):
        return np.array([[20, 20, 140, 140] + [60] * 10 + [0.99]], np.float32)

    def check_quality(self, *_args, **_options):
        pass

    def feature(self, frame, _face):
        vector = np.zeros(128, np.float32)
        vector[int(frame[0, 0, 0])] = 1
        return vector


@pytest.fixture
def dataset(tmp_path, monkeypatch):
    import base64

    monkeypatch.setattr("app.engine_tools.Pipeline", SyntheticPipeline)
    monkeypatch.setattr(
        "app.engine_tools.decode_image",
        lambda value: (
            np.full(
                (240, 240, 3),
                int(base64.b64decode(value).startswith(b"unknown")),
                np.uint8,
            ),
            b"",
        ),
    )
    filenames = (
        ["gallery"]
        + [f"known-{i}" for i in range(3)]
        + [f"unknown-{i}" for i in range(3)]
    )
    for name in filenames:
        (tmp_path / name).write_bytes(name.encode())
    data = {
        "split": "holdout",
        "model_sha256": SHA,
        "gallery": {"anon-one": ["gallery"]},
        "probes": [
            {"expected": "anon-one", "frames": filenames[1:4]},
            {"expected": None, "frames": filenames[4:]},
        ],
    }
    path = tmp_path / "data.json"
    path.write_text(json.dumps(data))
    return path, data


def test_evaluation_uses_observed_multiframe_decisions_without_attendance_approval(
    dataset,
):
    path, _ = dataset
    result = evaluate(path, path.parent, EngineConfig(0.8, 0.1), authorized=True)
    assert result["recognition_only"] and not result["automatic_attendance_approved"]
    assert result["reports"][0]["unknown_rejection_rate"] == 1
    assert result["reports"][0]["timings"]["embedding"]["samples"] == 6


class RejectingPAD:
    threshold = 0.99

    def __call__(self, *_):
        return False


def test_evaluation_runs_pad_and_counts_attack_rejections(dataset):
    path, data = dataset
    data["probes"][1]["attack"] = True
    path.write_text(json.dumps(data))
    result = evaluate(
        path,
        path.parent,
        EngineConfig(0.8, 0.1),
        authorized=True,
        liveness_hook=RejectingPAD(),
    )
    assert result["liveness_threshold"] == 0.99
    assert result["reports"][0]["attack_probes"] == 1
    assert result["reports"][0]["attack_pass_rate"] == 0
    assert result["reports"][0]["fnir"] == 1
    assert not result["automatic_attendance_approved"]


def test_evaluation_rejects_dataset_paths_outside_directory(dataset):
    path, data = dataset
    outside = path.parent.parent / "outside.jpg"
    outside.write_bytes(b"known outside")
    data["probes"][0]["frames"][0] = "../outside.jpg"
    path.write_text(json.dumps(data))
    with pytest.raises(ValueError, match="directory"):
        evaluate(path, path.parent, EngineConfig(0.8, 0.1), authorized=True)


def test_evaluation_rejects_repeated_probe_as_independent_evidence(dataset):
    path, data = dataset
    data["probes"].append(data["probes"][0])
    path.write_text(json.dumps(data))
    with pytest.raises(ValueError):
        evaluate(path, path.parent, EngineConfig(0.8, 0.1), authorized=True)


@pytest.mark.parametrize(
    "change",
    [
        "unauthorized",
        "wrong_split",
        "overlap",
        "duplicate",
        "model",
        "identity",
        "attack",
    ],
)
def test_evaluation_rejects_invalid_protocol_before_reporting_accuracy(dataset, change):
    path, data = dataset
    if change == "wrong_split":
        data["split"] = "calibration"
    if change == "overlap":
        data["probes"][0]["frames"][0] = "gallery"
    if change == "duplicate":
        data["probes"][0]["frames"][1] = data["probes"][0]["frames"][0]
    if change == "model":
        data["model_sha256"] = "b" * 64
    if change == "identity":
        data["probes"][0]["expected"] = "private student name"
    if change == "attack":
        data["probes"][0]["attack"] = True
    path.write_text(json.dumps(data))
    with pytest.raises(ValueError):
        evaluate(
            path,
            path.parent,
            EngineConfig(0.8, 0.1),
            authorized=change != "unauthorized",
        )
