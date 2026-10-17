"""Checksum-verified CPU face processing and encrypted enrollment."""

import base64
import binascii
import hashlib
import json
import os
import struct
import threading
from functools import lru_cache

import cv2
import numpy as np
from cryptography.hazmat.primitives.ciphers.aead import AESGCM

from app.models import MANIFEST, verify_model
from app.reference_photo import encrypt_reference_photo

EVALUATION = {
    "status": "not_evaluated",
    "required": ["camera_hold_out", "liveness_spoof", "threshold_calibration"],
    "automatic_attendance_approved": False,
}
MAX_IMAGE_BYTES = 384 * 1024


class ImageError(ValueError):
    def __init__(self, code, message):
        self.code, self.message = code, message
        super().__init__(message)


def jpeg_size(raw):
    """Inspect SOF dimensions before allocating a decoded image."""
    if len(raw) < 4 or raw[:2] != b"\xff\xd8" or raw[-2:] != b"\xff\xd9":
        raise ImageError(
            "INVALID_IMAGE", "Foto belum dapat diproses. Ambil ulang dari kamera."
        )
    offset = 2
    while offset + 4 <= len(raw):
        if raw[offset] != 255:
            break
        while offset < len(raw) and raw[offset] == 255:
            offset += 1
        if offset >= len(raw):
            break
        marker = raw[offset]
        offset += 1
        if marker in (0xD9, 0xDA):
            break
        if marker == 0x01 or 0xD0 <= marker <= 0xD7:
            continue
        if offset + 2 > len(raw):
            break
        size = int.from_bytes(raw[offset : offset + 2], "big")
        if size < 2 or offset + size > len(raw):
            break
        if marker in (0xC0, 0xC1, 0xC2):
            if size < 8 or raw[offset + 2] != 8 or raw[offset + 7] != 3:
                break
            height, width = struct.unpack(">HH", raw[offset + 3 : offset + 7])
            if not (160 <= width <= 1280 and 160 <= height <= 1280):
                raise ImageError(
                    "IMAGE_DIMENSIONS", "Ukuran JPEG harus 160–1280 piksel"
                )
            return width, height
        offset += size
    raise ImageError(
        "INVALID_IMAGE", "Foto belum dapat dibaca. Ambil ulang dari kamera."
    )


def decode_image(value):
    if not value or len(value) > ((MAX_IMAGE_BYTES + 2) // 3) * 4:
        raise ImageError(
            "IMAGE_TOO_LARGE", "Foto terlalu besar. Ambil ulang dari kamera."
        )
    try:
        raw = base64.b64decode(value, validate=True)
    except ValueError, binascii.Error:
        raise ImageError(
            "INVALID_IMAGE", "Gunakan base64 JPEG tanpa data URL"
        ) from None
    if len(raw) > MAX_IMAGE_BYTES:
        raise ImageError(
            "IMAGE_TOO_LARGE", "Foto terlalu besar. Ambil ulang dari kamera."
        )
    expected = jpeg_size(raw)
    frame = cv2.imdecode(np.frombuffer(raw, np.uint8), cv2.IMREAD_COLOR)
    if frame is None or (frame.shape[1], frame.shape[0]) != expected:
        raise ImageError(
            "INVALID_IMAGE", "Foto belum dapat dibaca. Ambil ulang dari kamera."
        )
    return frame, hashlib.sha256(raw).digest()


def normalize(vector):
    vector = np.asarray(vector, dtype=np.float32).reshape(-1)
    norm = float(np.linalg.norm(vector.astype(np.float64)))
    if (
        vector.size != 128
        or not np.isfinite(vector).all()
        or not np.isfinite(norm)
        or norm <= 0
    ):
        raise ValueError("Invalid embedding")
    return (vector.astype(np.float64) / norm).astype(np.float32)


def match_embedding(probe, gallery, *, threshold, margin, live, observations):
    """Offline exact matching; caller supplies evaluated thresholds and liveness evidence."""
    if not -1 <= threshold <= 1 or not 0 < margin <= 2:
        raise ValueError("Invalid calibrated thresholds")
    if live is not True or observations < 3 or not gallery:
        return None
    probe = normalize(probe)
    ranked = sorted(
        ((float(probe @ normalize(v)), subject) for subject, v in gallery.items()),
        reverse=True,
    )
    score, subject = ranked[0]
    runner_up = ranked[1][0] if len(ranked) > 1 else -1
    return subject if score >= threshold and score - runner_up >= margin else None


def aad(school_id, student_id, consent_id, model_sha256, key_id):
    return json.dumps(
        [
            "attendance-template-v1",
            school_id,
            student_id,
            consent_id,
            model_sha256,
            key_id,
        ],
        separators=(",", ":"),
    ).encode()


class TemplateCipher:
    def __init__(self, key, key_id):
        if len(key) != 32 or not key_id or len(key_id) > 80:
            raise ValueError("Invalid template key configuration")
        self.cipher, self.key_id = AESGCM(key), key_id

    def encrypt(self, vector, school_id, student_id, consent_id, model_sha256):
        nonce = os.urandom(12)
        associated = aad(school_id, student_id, consent_id, model_sha256, self.key_id)
        encrypted = self.cipher.encrypt(
            nonce, normalize(vector).astype("<f4").tobytes(), associated
        )
        return {
            "ciphertext": base64.b64encode(encrypted).decode(),
            "nonce": base64.b64encode(nonce).decode(),
            "key_id": self.key_id,
            "model_sha256": model_sha256,
        }

    def decrypt(self, template, school_id, student_id, consent_id):
        if template["key_id"] != self.key_id:
            raise ValueError("Unavailable template key")
        associated = aad(
            school_id, student_id, consent_id, template["model_sha256"], self.key_id
        )
        plaintext = self.cipher.decrypt(
            base64.b64decode(template["nonce"], validate=True),
            base64.b64decode(template["ciphertext"], validate=True),
            associated,
        )
        if len(plaintext) != 512:
            raise ValueError("Invalid template size")
        return normalize(np.frombuffer(plaintext, dtype="<f4"))


class Pipeline:
    def __init__(self, directory):
        manifest = json.loads(MANIFEST.read_text())
        specs = {spec["name"]: spec for spec in manifest["models"]}
        detector_spec, recognizer_spec = (
            specs["face_detection_yunet"],
            specs["face_recognition_sface"],
        )
        detector_path = verify_model(directory, detector_spec)
        recognizer_path = verify_model(directory, recognizer_spec)
        threads = int(os.environ.get("AI_OPENCV_THREADS", "2"))
        if not 1 <= threads <= 32:
            raise ValueError("Invalid OpenCV thread count")
        cv2.setNumThreads(threads)
        self.detector = cv2.FaceDetectorYN.create(
            str(detector_path), "", (320, 320), 0.9, 0.3, 100
        )
        self.recognizer = cv2.FaceRecognizerSF.create(str(recognizer_path), "")
        self.model_sha256 = recognizer_spec["sha256"]
        self.detector_sha256 = detector_spec["sha256"]
        self.lock = threading.Lock()

    def detect(self, frame, *, threshold=0.9):
        # OpenCV detector input size and inference buffers are mutable.
        with self.lock:
            self.detector.setScoreThreshold(threshold)
            self.detector.setInputSize((frame.shape[1], frame.shape[0]))
            _, faces = self.detector.detect(frame)
            return faces.copy() if faces is not None else np.empty((0, 15), np.float32)

    def feature(self, frame, face):
        with self.lock:
            aligned = self.recognizer.alignCrop(frame, face)
            return normalize(self.recognizer.feature(aligned).copy())

    @staticmethod
    def check_quality(frame, face, *, min_size=80):
        x, y, width, height = face[:4]
        if (
            min(width, height) < min_size
            or width * height / (frame.shape[0] * frame.shape[1]) < 0.04
        ):
            raise ImageError("FACE_TOO_SMALL", "Dekatkan wajah ke kamera")
        x0, y0 = max(0, int(x)), max(0, int(y))
        x1, y1 = (
            min(frame.shape[1], int(x + width)),
            min(frame.shape[0], int(y + height)),
        )
        crop = frame[y0:y1, x0:x1]
        if (
            crop.size == 0
            or float(
                cv2.Laplacian(cv2.cvtColor(crop, cv2.COLOR_BGR2GRAY), cv2.CV_64F).var()
            )
            < 60
        ):
            raise ImageError(
                "IMAGE_BLURRED", "Wajah buram; gunakan cahaya cukup dan tetap diam"
            )

    @staticmethod
    def boxes(frame, faces):
        boxes = []
        for face in faces:
            x, y, width, height = [float(v) for v in face[:4]]
            left, top = max(0, x / frame.shape[1]), max(0, y / frame.shape[0])
            right, bottom = (
                min(1, (x + width) / frame.shape[1]),
                min(1, (y + height) / frame.shape[0]),
            )
            if right > left and bottom > top:
                boxes.append(
                    {
                        "box": {
                            "x": left,
                            "y": top,
                            "width": right - left,
                            "height": bottom - top,
                        }
                    }
                )
        return boxes

    def preview(self, image):
        frame, _ = decode_image(image)
        faces = self.detect(frame)
        quality, message = "ready", "Wajah siap; ambil 3–5 frame yang berbeda"
        if len(faces) == 0:
            quality, message = "no_face", "Posisikan wajah di depan kamera"
        elif len(faces) > 1:
            quality, message = "multiple_faces", "Hanya satu orang di depan kamera"
        else:
            try:
                self.check_quality(frame, faces[0])
            except ImageError as exc:
                quality, message = "low_quality", exc.message
        return {
            "faces": self.boxes(frame, faces),
            "face_count": len(faces),
            "quality": quality,
            "message": message,
        }

    def enroll(
        self,
        frames,
        cipher,
        school_id,
        student_id,
        consent_id,
        *,
        gallery=(),
        duplicate_threshold=0.85,
    ):
        if not 3 <= len(frames) <= 5:
            raise ImageError("FRAME_COUNT", "Ambil 3–5 foto dari kamera.")
        vectors, seen = [], set()
        reference = None
        for value in frames:
            frame, digest = decode_image(value)
            if digest in seen:
                raise ImageError("DUPLICATE_FRAME", "Ambil foto kamera yang berbeda.")
            seen.add(digest)
            faces = self.detect(frame)
            if len(faces) != 1:
                raise ImageError(
                    "FACE_COUNT", "Pastikan setiap foto hanya menampilkan satu wajah."
                )
            self.check_quality(frame, faces[0])
            if reference is None:
                reference = (frame, faces[0])
            vector = self.feature(frame, faces[0])
            # shortcut: 0.6 is a provisional consistency filter, recalibrate with permitted camera data.
            if any(float(vector @ previous) < 0.6 for previous in vectors):
                raise ImageError(
                    "INCONSISTENT_IDENTITY",
                    "Foto menampilkan wajah yang berbeda. Ulangi pengambilan.",
                )
            vectors.append(vector)
        if not np.isfinite(duplicate_threshold) or not 0 < duplicate_threshold <= 1:
            raise ValueError("Invalid duplicate threshold")
        embedding = normalize(np.mean(vectors, axis=0))
        for template in gallery:
            if template["student_id"] == student_id:
                continue
            if template["model_sha256"] != self.model_sha256:
                raise ValueError("Incompatible gallery model")
            previous = cipher.decrypt(
                template, school_id, template["student_id"], template.get("consent_id")
            )
            if float(embedding @ previous) >= duplicate_threshold:
                raise ImageError(
                    "DUPLICATE_IDENTITY",
                    "Wajah ini sudah terdaftar. Periksa data siswa atau hubungi pengelola.",
                )
        result = cipher.encrypt(
            embedding,
            school_id,
            student_id,
            consent_id,
            self.model_sha256,
        )
        return dict(
            result,
            frame_count=len(vectors),
            status="provisional",
            reference_photo=encrypt_reference_photo(
                *reference, cipher, school_id, student_id
            ),
        )


@lru_cache(maxsize=1)
def _get_pipeline():
    return Pipeline(os.environ.get("AI_MODELS_DIR", "/models"))


_model_lock = threading.Lock()


def get_pipeline():
    # Serialize first use too: functools cache alone permits duplicate construction.
    with _model_lock:
        return _get_pipeline()


get_pipeline.cache_clear = _get_pipeline.cache_clear


def get_cipher():
    try:
        key = base64.b64decode(
            os.environ.get("AI_TEMPLATE_KEY_BASE64", ""), validate=True
        )
        return TemplateCipher(key, os.environ.get("AI_TEMPLATE_KEY_ID", ""))
    except ValueError, binascii.Error:
        raise ValueError("Template encryption key is unavailable") from None


def operation_mode():
    mode = os.environ.get("AI_OPERATION_MODE", "provisional").strip().lower()
    return (
        mode
        if mode in ("disabled", "shadow", "approved", "provisional")
        else "disabled"
    )


def evaluate_gate(service):
    mode = operation_mode()
    if mode == "disabled":
        return mode, False, "RECOGNITION_DISABLED"
    if not callable(getattr(service, "liveness_hook", None)):
        return mode, False, "PIPELINE_NOT_CONFIGURED"
    if mode == "shadow":
        return mode, False, "SHADOW_MODE"
    if mode == "approved":
        evaluation = getattr(service, "evaluation", None) or {}
        if evaluation.get("automatic_attendance_approved") is True:
            return mode, True, ""
        return mode, False, "APPROVAL_REQUIRED"
    return mode, True, ""


def status():
    mode = operation_mode()
    try:
        pipeline = get_pipeline()
        get_cipher()
    except OSError, ValueError, cv2.error:
        return {
            "pipeline_ready": False,
            "operation_mode": mode,
            "recognition_enabled": False,
            "block_reason": "PIPELINE_NOT_CONFIGURED",
            "evaluation": EVALUATION,
        }
    evaluation = EVALUATION
    enabled = False
    block_reason = "PIPELINE_NOT_CONFIGURED"
    try:
        from app.recognition import get_recognition_service

        service = get_recognition_service()
        evaluation = service.evaluation or EVALUATION
        mode, enabled, block_reason = evaluate_gate(service)
    except OSError, ValueError, cv2.error:
        pass
    return {
        "pipeline_ready": True,
        "operation_mode": mode,
        "recognition_enabled": enabled,
        "block_reason": block_reason,
        "model_sha256": pipeline.model_sha256,
        "detector_sha256": pipeline.detector_sha256,
        "evaluation": evaluation,
    }
