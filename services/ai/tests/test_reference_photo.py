import base64

import numpy as np
import pytest
from cryptography.exceptions import InvalidTag
from fastapi.testclient import TestClient

from app.main import app
from app.pipeline import TemplateCipher, decode_image
from app.reference_photo import decrypt_reference_photo, encrypt_reference_photo


def test_reference_is_one_bounded_jpeg_encrypted_and_bound_to_student_and_key():
    frame = np.random.default_rng(42).integers(0, 256, (480, 640, 3), dtype=np.uint8)
    face = np.array([150, 80, 170, 210], dtype=np.float32)
    cipher = TemplateCipher(b"k" * 32, "key")
    photo = encrypt_reference_photo(frame, face, cipher, "school", "student")
    assert set(photo) == {"ciphertext", "nonce", "key_id"}
    assert len(base64.b64decode(photo["ciphertext"])) <= 65552
    image = decrypt_reference_photo(photo, cipher, "school", "student")
    assert decode_image(image)[0].shape == (192, 192, 3)
    assert (
        photo["nonce"]
        != encrypt_reference_photo(frame, face, cipher, "school", "student")["nonce"]
    )
    for school, student in (("other", "student"), ("school", "other")):
        with pytest.raises(InvalidTag):
            decrypt_reference_photo(photo, cipher, school, student)
    for name in ("nonce", "ciphertext"):
        changed = dict(photo)
        raw = bytearray(base64.b64decode(photo[name]))
        raw[0] ^= 1
        changed[name] = base64.b64encode(raw).decode()
        with pytest.raises(InvalidTag):
            decrypt_reference_photo(changed, cipher, "school", "student")
    with pytest.raises(ValueError):
        decrypt_reference_photo(
            dict(photo, key_id="other"), cipher, "school", "student"
        )
    with pytest.raises(InvalidTag):
        decrypt_reference_photo(
            photo, TemplateCipher(b"x" * 32, "key"), "school", "student"
        )


def test_reference_endpoint_requires_internal_auth_and_does_not_echo_invalid_photo(
    monkeypatch,
):
    monkeypatch.setenv("AI_SERVICE_TOKEN", "s" * 48)
    with TestClient(app) as client:
        response = client.post("/internal/v1/reference-photo", json={})
        assert response.status_code == 401
        response = client.post(
            "/internal/v1/reference-photo",
            json={"ciphertext": "PRIVATE-IMAGE"},
            headers={"Authorization": "Bearer " + "s" * 48},
        )
        assert response.status_code == 422
        assert "PRIVATE-IMAGE" not in response.text


def test_reference_endpoint_decrypts_only_the_bound_student(monkeypatch):
    school = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
    student = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
    cipher = TemplateCipher(b"k" * 32, "key")
    photo = encrypt_reference_photo(
        np.zeros((240, 240, 3), np.uint8), [30, 30, 160, 160], cipher, school, student
    )
    monkeypatch.setenv("AI_SERVICE_TOKEN", "s" * 48)
    monkeypatch.setattr("app.main.get_cipher", lambda: cipher)
    body = dict(photo, school_id=school, student_id=student)
    headers = {"Authorization": "Bearer " + "s" * 48}
    with TestClient(app) as client:
        response = client.post(
            "/internal/v1/reference-photo", json=body, headers=headers
        )
        assert response.status_code == 200
        assert response.headers["Cache-Control"] == "no-store"
        assert decode_image(response.json()["image"])[0].shape == (192, 192, 3)
        response = client.post(
            "/internal/v1/reference-photo",
            json=dict(body, student_id=school),
            headers=headers,
        )
        assert response.status_code == 422
        assert "image" not in response.json()
        assert photo["ciphertext"] not in response.text
