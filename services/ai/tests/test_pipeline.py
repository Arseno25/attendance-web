import base64
from pathlib import Path

import cv2
import numpy as np
import pytest
from cryptography.exceptions import InvalidTag
from fastapi.testclient import TestClient

from app.main import app
from app.pipeline import (
    ImageError,
    Pipeline,
    TemplateCipher,
    decode_image,
    match_embedding,
)


def image(seed, size=240, blurred=False):
    pixels = np.random.default_rng(seed).integers(
        0, 256, (size, size, 3), dtype=np.uint8
    )
    if blurred:
        pixels[:] = seed
    ok, encoded = cv2.imencode(".jpg", pixels)
    assert ok
    return base64.b64encode(encoded).decode()


def vector(index=0):
    value = np.zeros(128, np.float32)
    value[index] = 1
    return value


def fake_pipeline(monkeypatch, faces=1, small=False):
    pipeline = object.__new__(Pipeline)
    pipeline.model_sha256 = "a" * 64
    row = np.array([30, 30, 20 if small else 160, 160] + [50] * 10 + [0.99], np.float32)
    monkeypatch.setattr(
        pipeline, "detect", lambda frame, **kwargs: np.tile(row, (faces, 1))
    )
    monkeypatch.setattr(pipeline, "feature", lambda frame, face: vector())
    return pipeline


def test_cipher_roundtrip_random_nonce_and_all_aad_bindings():
    cipher = TemplateCipher(b"k" * 32, "test-key")
    values = ("school", "student", "consent", "a" * 64)
    encrypted = cipher.encrypt(vector(), *values)
    assert encrypted["nonce"] != cipher.encrypt(vector(), *values)["nonce"]
    assert np.allclose(cipher.decrypt(encrypted, *values[:3]), vector())
    for i in range(3):
        changed = list(values[:3])
        changed[i] += "different"
        with pytest.raises(InvalidTag):
            cipher.decrypt(encrypted, *changed)
    for field in ("nonce", "ciphertext"):
        tampered = dict(encrypted)
        raw = bytearray(base64.b64decode(tampered[field]))
        raw[0] ^= 1
        tampered[field] = base64.b64encode(raw).decode()
        with pytest.raises(InvalidTag):
            cipher.decrypt(tampered, *values[:3])
    with pytest.raises(InvalidTag):
        cipher.decrypt(dict(encrypted, model_sha256="b" * 64), *values[:3])
    with pytest.raises(ValueError):
        cipher.decrypt(dict(encrypted, key_id="other"), *values[:3])


@pytest.mark.parametrize(
    "value", ["", "data:image/jpeg;base64,AA==", "not a jpeg", "A" * 524292]
)
def test_invalid_and_large_images_are_rejected(value):
    with pytest.raises(ImageError):
        decode_image(value)


def test_image_dimensions_checked_before_decoder(monkeypatch):
    jpeg = bytearray(base64.b64decode(image(1)))
    at = jpeg.find(b"\xff\xc0")
    assert at != -1
    jpeg[at + 5 : at + 7] = (64000).to_bytes(2, "big")
    monkeypatch.setattr(
        cv2, "imdecode", lambda *args: pytest.fail("decoded before dimensions checked")
    )
    with pytest.raises(ImageError, match="Ukuran"):
        decode_image(base64.b64encode(jpeg).decode())


@pytest.mark.parametrize(
    "faces,blurred,small,code",
    [
        (0, False, False, "FACE_COUNT"),
        (2, False, False, "FACE_COUNT"),
        (1, True, False, "IMAGE_BLURRED"),
        (1, False, True, "FACE_TOO_SMALL"),
    ],
)
def test_enrollment_quality_rejections(monkeypatch, faces, blurred, small, code):
    pipeline = fake_pipeline(monkeypatch, faces, small)
    with pytest.raises(ImageError) as raised:
        pipeline.enroll(
            [image(i, blurred=blurred) for i in (1, 2, 3)],
            TemplateCipher(b"k" * 32, "key"),
            "s",
            "st",
            "co",
        )
    assert raised.value.code == code


def test_enrollment_consistency_duplicate_and_ciphertext_only(monkeypatch):
    pipeline = fake_pipeline(monkeypatch)
    frames = [image(i) for i in (1, 2, 3)]
    cipher = TemplateCipher(b"k" * 32, "key")
    result = pipeline.enroll(frames, cipher, "s", "st", "co")
    assert result["frame_count"] == 3 and result["status"] == "provisional"
    assert "embedding" not in result and "frames" not in result
    assert np.allclose(cipher.decrypt(result, "s", "st", "co"), vector())
    with pytest.raises(ImageError, match="berbeda"):
        pipeline.enroll([frames[0]] * 3, cipher, "s", "st", "co")
    values = iter([vector(0), vector(1)])
    monkeypatch.setattr(pipeline, "feature", lambda frame, face: next(values))
    with pytest.raises(ImageError, match="konsisten"):
        pipeline.enroll(frames, cipher, "s", "st", "co")


def test_matching_unknown_spoof_ambiguity_and_stability_cannot_identify():
    gallery = {"anonymous-a": vector(0), "anonymous-b": vector(1)}
    options = {"threshold": 0.9, "margin": 0.1, "live": True, "observations": 3}
    assert match_embedding(vector(0), gallery, **options) == "anonymous-a"
    assert match_embedding(vector(2), gallery, **options) is None
    assert match_embedding(vector(0), gallery, **dict(options, live=False)) is None
    assert match_embedding(vector(0), gallery, **dict(options, observations=2)) is None
    assert (
        match_embedding(vector(0), dict(gallery, duplicate=vector(0)), **options)
        is None
    )


def test_detection_returns_only_boxes_and_honest_gate(monkeypatch):
    pipeline = fake_pipeline(monkeypatch)
    result = pipeline.recognize(image(1))
    assert result["decision"] == "unknown" and result["recognition_enabled"] is False
    assert len(result["faces"]) == 1
    assert list(result["faces"][0]) == ["box"]
    assert all(0 <= v <= 1 for v in result["faces"][0]["box"].values())
    assert result["block_reason"] == "MODEL_LIVENESS_NOT_EVALUATED"


def test_internal_body_limits_and_validation_do_not_echo_images(monkeypatch):
    monkeypatch.setenv("AI_SERVICE_TOKEN", "a" * 48)
    with TestClient(app) as client:
        result = client.post(
            "/internal/v1/enroll",
            content=b"x" * ((2 << 20) + 1),
            headers={
                "Authorization": "Bearer " + "a" * 48,
                "Content-Type": "application/json",
            },
        )
        assert result.status_code == 413
        result = client.post(
            "/internal/v1/enroll",
            json={"frames": ["PRIVATE-IMAGE"]},
            headers={"Authorization": "Bearer " + "a" * 48},
        )
        assert result.status_code == 422 and "PRIVATE-IMAGE" not in result.text


def test_actual_pinned_cpu_models_smoke_without_biometric_data():
    directory = Path(__file__).resolve().parents[3] / ".tools" / "models"
    if not directory.exists():
        pytest.skip("pinned model artifacts not present")
    pipeline = Pipeline(directory)
    blank = np.zeros((320, 320, 3), np.uint8)
    assert len(pipeline.detect(blank)) == 0
    # Synthetic noise and landmarks exercise SFace allocation without a person's image.
    synthetic = np.random.default_rng(7).integers(0, 256, (320, 320, 3), dtype=np.uint8)
    face = np.array(
        [70, 70, 150, 180, 110, 125, 180, 125, 145, 160, 120, 200, 175, 200, 0.99],
        np.float32,
    )
    feature = pipeline.feature(synthetic, face)
    assert feature.shape == (128,) and np.isfinite(feature).all()
    assert float(np.linalg.norm(feature)) == pytest.approx(1, abs=0.0001)
