"""Bounded, school/device-isolated inference using the existing recognition engine."""

import hashlib
import json
import os
import threading
from collections import OrderedDict
from functools import lru_cache

from app.approval import load_approval
from app.face_engine import EngineConfig, FaceRecognitionEngine
from app.pipeline import ImageError, decode_image, get_cipher, get_pipeline


class RecognitionService:
    def __init__(
        self, pipeline, cipher, config, *, liveness_hook=None, evaluation=None
    ):
        self.pipeline, self.cipher, self.config = pipeline, cipher, config
        self.liveness_hook = liveness_hook
        self.evaluation = evaluation
        self._contexts = OrderedDict()
        self._frames = OrderedDict()
        self._lock = threading.Lock()

    def process(self, image, *, school_id, session_id, device_id, track_id, gallery):
        if len(gallery) > 3000:
            raise ValueError("Gallery capacity exceeded")
        frame, _ = decode_image(image)
        key = (school_id, session_id, device_id)
        revision = hashlib.sha256(
            json.dumps(gallery, sort_keys=True, separators=(",", ":")).encode()
        ).digest()
        with self._lock:
            # Preserve recent decoded fingerprints across track and gallery resets.
            seen = self._frames.pop(key, OrderedDict())
            digest = hashlib.sha256(frame.tobytes()).digest()
            duplicate = digest in seen
            seen[digest] = None
            while len(seen) > 2048:
                seen.popitem(last=False)
            self._frames[key] = seen
            while len(self._frames) > 8:
                self._frames.popitem(last=False)
            if duplicate:
                self._contexts.pop(key, None)
                raise ImageError(
                    "CAPTURE_REPLAY", "Foto kamera sudah digunakan. Ambil gambar baru."
                )
            cached = self._contexts.pop(key, None)
            if cached is None or cached[0] != revision:
                engine = FaceRecognitionEngine(
                    self.config,
                    pipeline=self.pipeline,
                    liveness_hook=self.liveness_hook,
                )
                engine.load_encrypted_templates(
                    gallery, self.cipher, school_id=school_id
                )
            else:
                engine = cached[1]
            self._contexts[key] = (revision, engine)
            while len(self._contexts) > 8:
                self._contexts.popitem(last=False)
            return engine.process_frame(frame, track_id)


@lru_cache(maxsize=1)
def _get_recognition_service():
    from app.liveness import LivenessModel

    pipeline = get_pipeline()
    config = EngineConfig(
        threshold=float(os.getenv("AI_MATCH_THRESHOLD", "0.8")),
        margin=float(os.getenv("AI_MATCH_MARGIN", "0.1")),
    )
    liveness = LivenessModel(
        os.getenv("AI_MODELS_DIR", "/models"),
        threshold=float(os.getenv("AI_LIVENESS_THRESHOLD", "0.95")),
    )
    evaluation = load_approval(
        os.getenv("AI_RECOGNITION_APPROVAL_PATH"),
        os.getenv("AI_RECOGNITION_APPROVAL_SHA256", ""),
        pipeline.model_sha256,
        liveness.model_sha256,
        config,
        liveness.threshold,
        pipeline.detector_sha256,
    )
    return RecognitionService(
        pipeline, get_cipher(), config, liveness_hook=liveness, evaluation=evaluation
    )


_service_lock = threading.Lock()


def get_recognition_service():
    with _service_lock:
        return _get_recognition_service()
