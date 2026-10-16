"""One small, encrypted registration portrait; never a recognition input."""

import base64
import json
import os

import cv2

MAX_PHOTO_BYTES = 65536


def _aad(school_id, student_id, key_id):
    return json.dumps(
        ["attendance-reference-photo-v1", school_id, student_id, key_id],
        separators=(",", ":"),
    ).encode()


def encrypt_reference_photo(frame, face, cipher, school_id, student_id):
    x, y, width, height = map(float, face[:4])
    side = min(round(max(width, height) * 1.4), *frame.shape[:2])
    if side <= 0:
        raise ValueError("Invalid reference photo bounds")
    left = max(0, min(round(x + width / 2 - side / 2), frame.shape[1] - side))
    top = max(0, min(round(y + height / 2 - side / 2), frame.shape[0] - side))
    portrait = cv2.resize(frame[top : top + side, left : left + side], (192, 192))
    ok, encoded = cv2.imencode(".jpg", portrait, [cv2.IMWRITE_JPEG_QUALITY, 82])
    if not ok or encoded.size > MAX_PHOTO_BYTES:
        raise ValueError("Reference photo exceeds limit")
    nonce = os.urandom(12)
    encrypted = cipher.cipher.encrypt(
        nonce, encoded.tobytes(), _aad(school_id, student_id, cipher.key_id)
    )
    return {
        "ciphertext": base64.b64encode(encrypted).decode(),
        "nonce": base64.b64encode(nonce).decode(),
        "key_id": cipher.key_id,
    }


def decrypt_reference_photo(photo, cipher, school_id, student_id):
    if photo["key_id"] != cipher.key_id:
        raise ValueError("Reference photo key mismatch")
    encrypted = base64.b64decode(photo["ciphertext"], validate=True)
    nonce = base64.b64decode(photo["nonce"], validate=True)
    if len(nonce) != 12 or not 16 < len(encrypted) <= MAX_PHOTO_BYTES + 16:
        raise ValueError("Invalid encrypted reference photo")
    raw = cipher.cipher.decrypt(
        nonce, encrypted, _aad(school_id, student_id, cipher.key_id)
    )
    return base64.b64encode(raw).decode()
