"""Synthetic identities prove integration and isolation, never camera accuracy."""

from uuid import uuid4

import pytest
from cryptography.exceptions import InvalidTag
from fastapi.testclient import TestClient
from test_pipeline import fake_pipeline, image, vector

from app import main
from app.face_engine import EngineConfig
from app.pipeline import ImageError, TemplateCipher


def template(cipher, school, student, value=None):
    return dict(
        cipher.encrypt(
            vector() if value is None else value, school, student, None, "a" * 64
        ),
        student_id=student,
        consent_id=None,
    )


def test_enrollment_duplicate_check_is_cross_student_but_not_cross_school(monkeypatch):
    pipeline = fake_pipeline(monkeypatch)
    cipher = TemplateCipher(b"k" * 32, "test-key")
    frames = [image(i) for i in (1, 2, 3)]
    gallery = [template(cipher, "school", "other")]
    with pytest.raises(ImageError) as error:
        pipeline.enroll(frames, cipher, "school", "new", None, gallery=gallery)
    assert error.value.code == "DUPLICATE_IDENTITY"
    own = pipeline.enroll(frames, cipher, "school", "other", None, gallery=gallery)
    assert own["status"] == "provisional"
    with pytest.raises(InvalidTag):
        pipeline.enroll(frames, cipher, "another-school", "new", None, gallery=gallery)


def test_http_recognition_uses_engine_but_hides_unevaluated_identity(monkeypatch):
    from app.recognition import RecognitionService

    pipeline = fake_pipeline(monkeypatch)
    cipher = TemplateCipher(b"k" * 32, "test-key")
    service = RecognitionService(
        pipeline,
        cipher,
        EngineConfig(threshold=0.8, margin=0.1, min_interval=0, cooldown=0),
        liveness_hook=lambda *_: True,
    )
    monkeypatch.setattr(main, "get_recognition_service", lambda: service)
    monkeypatch.setenv("AI_SERVICE_TOKEN", "a" * 48)
    school, student = str(uuid4()), str(uuid4())
    body = {
        "school_id": school,
        "session_id": str(uuid4()),
        "device_id": str(uuid4()),
        "track_id": str(uuid4()),
        "event_id": str(uuid4()),
        "image": image(1),
        "gallery_revision": 1,
        "gallery": [template(cipher, school, student)],
    }
    with TestClient(main.app) as client:
        statuses = []
        for _ in range(3):
            body["event_id"] = str(uuid4())
            response = client.post(
                "/internal/v1/recognize",
                json=body,
                headers={"Authorization": "Bearer " + "a" * 48},
            )
            assert response.status_code == 200
            data = response.json()
            statuses.append(data["engine_status"])
            assert data["recognition_enabled"] is False
            assert data["decision"] == "unknown"
            assert student not in response.text
            assert len(data["faces"]) == 1 and list(data["faces"][0]) == ["box"]
        assert statuses == ["PROCESSING", "PROCESSING", "CONFIRMED"]


def test_service_resets_on_gallery_change_and_separates_devices(monkeypatch):
    from app.recognition import RecognitionService

    cipher = TemplateCipher(b"k" * 32, "test-key")
    service = RecognitionService(
        fake_pipeline(monkeypatch),
        cipher,
        EngineConfig(threshold=0.8, margin=0.1, min_interval=0, cooldown=0),
        liveness_hook=lambda *_: True,
    )
    gallery = [template(cipher, "school", "student")]
    args = {
        "school_id": "school",
        "session_id": "session",
        "device_id": "one",
        "track_id": "track",
        "gallery": gallery,
    }
    for _ in range(3):
        result = service.process(image(1), **args)
    assert result.status == "CONFIRMED"
    assert (
        service.process(image(1), **dict(args, device_id="two")).status == "PROCESSING"
    )
    assert service.process(image(1), **dict(args, gallery=[])).status == "UNKNOWN"
    bad = [dict(gallery[0], ciphertext="invalid")]
    with pytest.raises((ValueError, InvalidTag)):
        service.process(image(1), **dict(args, gallery=bad))
    assert service.process(image(1), **args).status == "PROCESSING"


def test_engine_pad_failure_clears_candidate_and_box_is_normalized(monkeypatch):
    from app.recognition import RecognitionService

    cipher = TemplateCipher(b"k" * 32, "test-key")
    service = RecognitionService(
        fake_pipeline(monkeypatch),
        cipher,
        EngineConfig(threshold=0.8, margin=0.1, min_interval=0),
        liveness_hook=lambda *_: False,
    )
    args = {
        "school_id": "school",
        "session_id": "session",
        "device_id": "one",
        "track_id": "track",
        "gallery": [template(cipher, "school", "student")],
    }
    for _ in range(3):
        result = service.process(image(1), **args)
        assert result.student_id is None and result.liveness_passed is False
        assert result.status == "UNKNOWN"
        assert all(0 <= n <= 1 for n in result.faces[0]["box"].values())


def test_enrollment_internal_limit_accepts_full_bounded_gallery(monkeypatch):
    cipher = TemplateCipher(b"k" * 32, "test-key")
    monkeypatch.setattr(main, "get_pipeline", lambda: fake_pipeline(monkeypatch))
    monkeypatch.setattr(main, "get_cipher", lambda: cipher)
    monkeypatch.setenv("AI_SERVICE_TOKEN", "a" * 48)
    school, student = str(uuid4()), str(uuid4())
    body = {
        "school_id": school,
        "student_id": student,
        "frames": [image(i) for i in (1, 2, 3)],
        "gallery": [template(cipher, school, student)] * 2500,
    }
    with TestClient(main.app) as client:
        response = client.post(
            "/internal/v1/enroll",
            json=body,
            headers={"Authorization": "Bearer " + "a" * 48},
        )
    assert response.status_code == 200


@pytest.mark.parametrize("live", [True, False])
def test_approved_route_still_requires_live_multiframe_confirmation(monkeypatch, live):
    from app.recognition import RecognitionService

    cipher = TemplateCipher(b"k" * 32, "test-key")
    evaluation = {
        key: True
        for key in (
            "automatic_attendance_approved",
            "holdout_verified",
            "liveness_verified",
            "stability_verified",
            "threshold_margin_verified",
        )
    }
    evaluation.update(model_sha256="a" * 64, report_sha256="b" * 64)
    service = RecognitionService(
        fake_pipeline(monkeypatch),
        cipher,
        EngineConfig(0.8, 0.1, min_interval=0, cooldown=0),
        liveness_hook=lambda *_: live,
        evaluation=evaluation,
    )
    monkeypatch.setattr(main, "get_recognition_service", lambda: service)
    monkeypatch.setenv("AI_SERVICE_TOKEN", "a" * 48)
    school, student = str(uuid4()), str(uuid4())
    body = {
        "school_id": school,
        "session_id": str(uuid4()),
        "device_id": str(uuid4()),
        "track_id": str(uuid4()),
        "event_id": str(uuid4()),
        "image": image(1),
        "gallery_revision": 1,
        "gallery": [template(cipher, school, student)],
    }
    with TestClient(main.app) as client:
        for i in range(3):
            response = client.post(
                "/internal/v1/recognize",
                json=body,
                headers={"Authorization": "Bearer " + "a" * 48},
            )
            assert response.status_code == 200
            data = response.json()
            if i == 2 and live:
                assert data["decision"] == "accepted"
                assert data["recognition_enabled"] is True
                assert data["faces"][0]["student_id"] == student
                assert data["evaluation"]["report_sha256"] == "b" * 64
            else:
                assert data["decision"] != "accepted" and student not in response.text


def test_status_follows_reviewed_service_policy(monkeypatch):
    from app import pipeline as module
    from app import recognition
    from app.recognition import RecognitionService

    pipeline = fake_pipeline(monkeypatch)
    pipeline.detector_sha256 = "c" * 64
    cipher = TemplateCipher(b"k" * 32, "test-key")
    service = RecognitionService(
        pipeline,
        cipher,
        EngineConfig(0.8, 0.1),
        evaluation={"automatic_attendance_approved": True},
    )
    monkeypatch.setattr(module, "get_pipeline", lambda: pipeline)
    monkeypatch.setattr(module, "get_cipher", lambda: cipher)
    monkeypatch.setattr(recognition, "get_recognition_service", lambda: service)
    assert module.status()["recognition_enabled"] is True
    service.evaluation = None
    assert module.status()["recognition_enabled"] is False
