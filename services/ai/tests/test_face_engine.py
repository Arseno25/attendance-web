"""Synthetic vectors test correctness, never biometric recognition accuracy."""

import time
from dataclasses import replace
from pathlib import Path

import cv2
import numpy as np
import pytest
from fastapi.testclient import TestClient
from test_pipeline import fake_pipeline, image, vector

from app.face_engine import (
    EngineConfig,
    FaceMatcher,
    FaceRecognitionEngine,
    RecognitionSession,
)
from app.main import app
from app.pipeline import Pipeline, TemplateCipher

SHA = "a" * 64
CONFIG = EngineConfig(threshold=0.8, margin=0.1, min_interval=0, cooldown=0)


def matcher(gallery=None, config=CONFIG):
    result = FaceMatcher(SHA, config)
    result.load_embeddings(gallery or {}, model_sha256=SHA)
    return result


def test_vectorized_multitemplate_distinct_identity_top2_and_unknown():
    match = matcher({"a": [vector(0), vector(1)], "b": [vector(2)]})
    known = match.match(vector(1))
    assert known.student_id == "a" and known.similarity == pytest.approx(1)
    assert len(known.candidates) == 2
    assert match.match(vector(3)).student_id is None
    match.update_student_embeddings("b", [vector(1)], model_sha256=SHA)
    assert match.match(vector(1)).student_id is None  # indistinguishable identities
    match.remove_student("b")
    assert match.match(vector(1)).student_id == "a"
    match.remove_student("a")
    assert match.match(vector(1)).similarity is None
    blend = vector(0) * 0.7 + vector(3) * 0.714
    assert matcher({"a": [vector(0)]}).match(blend).student_id is None


@pytest.mark.parametrize(
    "bad",
    [
        [np.ones(127)],
        [np.zeros(128)],
        [np.full(128, np.nan)],
        [np.full(128, np.inf)],
        [np.ones(128)] * 6,
        np.ones(128),
    ],
)
def test_gallery_rejects_invalid_vectors_transactionally(bad):
    match = matcher({"a": [vector(0)]})
    with pytest.raises(ValueError):
        match.load_embeddings({"b": bad}, model_sha256=SHA)
    assert match.match(vector(0)).student_id == "a"
    with pytest.raises(ValueError):
        match.load_embeddings({"b": [vector()]}, model_sha256="b" * 64)


def test_session_confirmation_identity_change_unknown_and_reset():
    match = matcher({"a": [vector(0)], "b": [vector(1)]})
    session = RecognitionSession(CONFIG)
    for i in range(2):
        assert not session.observe(match.match(vector(0)), vector(0), i)
    assert session.observe(match.match(vector(0)), vector(0), 2)
    assert not session.observe(match.match(vector(1)), vector(1), 3)
    assert len(session.matches) == 1
    assert not session.observe(match.match(vector(2)), vector(2), 4)
    assert not session.matches and session.state == "UNKNOWN"
    session.reset()
    assert session.state == "RESET"


def engine(monkeypatch, config=CONFIG):
    pipeline = fake_pipeline(monkeypatch)
    result = FaceRecognitionEngine(config, pipeline=pipeline)
    result.load_embeddings({"a": [vector(0)], "b": [vector(1)]}, model_sha256=SHA)
    frame = np.random.default_rng(7).integers(0, 256, (240, 240, 3), dtype=np.uint8)
    return result, frame


def test_engine_confirms_only_three_frames_and_does_not_reuse_disappeared_person(
    monkeypatch,
):
    result, frame = engine(monkeypatch)
    assert [result.process_frame(frame, "camera").status for _ in range(3)] == [
        "PROCESSING",
        "PROCESSING",
        "CONFIRMED",
    ]
    missing = result.pipeline.detect
    monkeypatch.setattr(
        result.pipeline, "detect", lambda _, **kwargs: np.empty((0, 15), np.float32)
    )
    assert result.process_frame(frame, "camera").status == "NO_FACE"
    monkeypatch.setattr(result.pipeline, "detect", missing)
    assert result.process_frame(frame, "camera").student_id is None
    monkeypatch.setattr(result.pipeline, "feature", lambda *_: vector(1))
    assert result.process_frame(frame, "camera").student_id is None
    assert result.process_frame(frame, "camera").student_id is None
    confirmed = result.process_frame(frame, "camera")
    assert confirmed.student_id == "b" and confirmed.recognition_only
    assert confirmed.liveness_passed is None
    result.remove_student("b")
    assert result.process_frame(frame, "camera").status == "UNKNOWN"


def test_engine_multiple_faces_quality_exceptions_stale_and_bounded_sessions(
    monkeypatch,
):
    result, frame = engine(monkeypatch, replace(CONFIG, max_sessions=2))
    detect = result.pipeline.detect
    monkeypatch.setattr(
        result.pipeline,
        "detect",
        lambda f, **kwargs: np.tile(detect(f, **kwargs), (2, 1)),
    )
    assert result.process_frame(frame, "camera").status == "MULTIPLE_FACES"
    monkeypatch.setattr(result.pipeline, "detect", detect)
    assert result.process_frame(np.zeros_like(frame), "camera").status == "LOW_QUALITY"
    before = time.monotonic() - 0.1
    assert (
        result.process_frame(frame, "camera", timestamp=before).reason == "STALE_FRAME"
    )
    assert (
        result.process_frame(frame, "camera", timestamp=time.monotonic() - 10).reason
        == "STALE_FRAME"
    )
    for key in ("one", "two", "three"):
        result.process_frame(frame, key)
    assert len(result._sessions) == 2
    assert "one" not in result._sessions
    monkeypatch.setattr(
        result.pipeline,
        "feature",
        lambda *_: (_ for _ in ()).throw(ValueError("private inference error")),
    )
    failed = result.process_frame(frame, "three")
    assert failed.status == "ERROR" and failed.reason == "INFERENCE_FAILED"
    assert "private" not in repr(failed)


def test_cooldown_and_sampling_never_echo_cached_identity(monkeypatch):
    result, frame = engine(monkeypatch, replace(CONFIG, cooldown=10))
    for _ in range(3):
        confirmed = result.process_frame(frame, "camera")
    assert confirmed.status == "CONFIRMED"
    monkeypatch.setattr(
        result.pipeline, "feature", lambda *_: pytest.fail("cooldown extracted")
    )
    cooled = result.process_frame(frame, "camera")
    assert (
        cooled.status == "PROCESSING"
        and cooled.student_id is None
        and not cooled.is_confirmed
    )
    result._slot.acquire()
    try:
        assert result.process_frame(frame, "camera").reason == "INFERENCE_BUSY"
    finally:
        result._slot.release()


def test_gallery_decryption_preserves_consent_binding_and_model(monkeypatch):
    result, _ = engine(monkeypatch)
    cipher = TemplateCipher(b"k" * 32, "key")
    template = cipher.encrypt(vector(0), "school", "a", "consent", SHA)
    template.update(student_id="a", consent_id="consent")
    result.load_encrypted_templates([template], cipher, school_id="school")
    assert result.matcher.match(vector(0)).student_id == "a"
    from cryptography.exceptions import InvalidTag

    with pytest.raises(InvalidTag):
        result.load_encrypted_templates(
            [dict(template, consent_id="revoked")], cipher, school_id="school"
        )
    with pytest.raises(ValueError):
        result.load_encrypted_templates(
            [dict(template, model_sha256="b" * 64)], cipher, school_id="school"
        )


@pytest.mark.parametrize(
    "faces,blurred,small,quality",
    [
        (0, False, False, "no_face"),
        (2, False, False, "multiple_faces"),
        (1, True, False, "low_quality"),
        (1, False, True, "low_quality"),
        (1, False, False, "ready"),
    ],
)
def test_preview_shares_quality_and_never_extracts_embedding(
    monkeypatch, faces, blurred, small, quality
):
    pipeline = fake_pipeline(monkeypatch, faces, small)
    monkeypatch.setattr(
        pipeline, "feature", lambda *_: pytest.fail("preview extracted embedding")
    )
    preview = pipeline.preview(image(1, blurred=blurred))
    assert set(preview) == {"faces", "face_count", "quality", "message"}
    assert preview["quality"] == quality and preview["face_count"] == faces
    assert all(set(row) == {"box"} for row in preview["faces"])


def test_preview_auth_strict_body_limit_and_errors_do_not_echo(monkeypatch):
    monkeypatch.setenv("AI_SERVICE_TOKEN", "a" * 48)
    pipeline = fake_pipeline(monkeypatch)
    monkeypatch.setattr("app.main.get_pipeline", lambda: pipeline)
    headers = {"Authorization": "Bearer " + "a" * 48}
    with TestClient(app) as client:
        assert (
            client.post(
                "/internal/v1/face-preview", json={"image": image(1)}
            ).status_code
            == 401
        )
        response = client.post(
            "/internal/v1/face-preview", json={"image": image(1)}, headers=headers
        )
        assert response.status_code == 200 and response.json()["quality"] == "ready"
        assert response.headers["cache-control"] == "no-store"
        response = client.post(
            "/internal/v1/face-preview",
            content=b"x" * ((512 << 10) + 1),
            headers=headers,
        )
        assert response.status_code == 413
        response = client.post(
            "/internal/v1/face-preview",
            json={"image": "private", "student_id": "secret"},
            headers=headers,
        )
        assert response.status_code == 422 and "secret" not in response.text
        response = client.post(
            "/internal/v1/face-preview", json={"image": "private"}, headers=headers
        )
        assert response.status_code == 422 and "private" not in response.text


def test_cpu_engine_model_smoke_is_not_accuracy_test():
    directory = Path(__file__).resolve().parents[3] / ".tools" / "models"
    if not directory.exists():
        pytest.skip("pinned models unavailable")
    pipeline = Pipeline(directory)
    result = FaceRecognitionEngine(CONFIG, pipeline=pipeline)
    assert (
        result.process_frame(np.zeros((320, 320, 3), np.uint8), "synthetic").status
        == "NO_FACE"
    )
    assert isinstance(result.pipeline.detector, cv2.FaceDetectorYN)


@pytest.mark.parametrize(
    "options",
    [
        {"threshold": float("nan")},
        {"margin": 0},
        {"consistent_matches": 6},
        {"consistent_matches": 1},
        {"confirmation_window": 5.5},
        {"detection_threshold": 0},
        {"max_sessions": 0},
        {"min_interval": -1},
    ],
)
def test_configuration_bounds(options):
    with pytest.raises(ValueError):
        replace(CONFIG, **options)


@pytest.mark.parametrize("outcome", [False, "invalid", KeyError("private")])
def test_pad_failure_resets_session_and_never_confirms(monkeypatch, outcome):
    result, frame = engine(monkeypatch)
    result.process_frame(frame, "camera")
    result.process_frame(frame, "camera")

    def hook(*_):
        if isinstance(outcome, Exception):
            raise outcome
        return outcome

    result.liveness_hook = hook
    failed = result.process_frame(frame, "camera")
    assert failed.status == ("UNKNOWN" if outcome is False else "ERROR")
    assert failed.student_id is None and not failed.is_confirmed
    assert not result._sessions["camera"].matches
    assert "private" not in repr(failed)
    result.liveness_hook = lambda *_: True
    assert result.process_frame(frame, "camera").status == "PROCESSING"


@pytest.mark.parametrize("invalid", ["frame", "expired", "replayed"])
def test_invalid_or_stale_frame_clears_pending_confirmation(monkeypatch, invalid):
    result, frame = engine(monkeypatch)
    result.process_frame(frame, "camera")
    result.process_frame(frame, "camera")
    timestamp = result._sessions["camera"].last_timestamp
    failed = result.process_frame(
        np.zeros((1, 1, 3), np.uint8) if invalid == "frame" else frame,
        "camera",
        timestamp=timestamp - 10
        if invalid == "expired"
        else timestamp
        if invalid == "replayed"
        else None,
    )
    assert failed.status == "ERROR"
    assert not result._sessions["camera"].matches
    assert result._sessions["camera"].last_timestamp == timestamp
    assert result.process_frame(frame, "camera").status == "PROCESSING"
