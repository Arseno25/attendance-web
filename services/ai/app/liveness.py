"""Pinned MiniFASNet PAD inference. Scores need camera-specific calibration."""

import json
import math

import cv2
import numpy as np
import onnxruntime as ort

from app.models import MANIFEST, verify_model


def prepare_crop(frame, box):
    height, width = frame.shape[:2]
    x, y, w, h = map(float, box[:4])
    if not all(math.isfinite(n) for n in (x, y, w, h)) or min(w, h) <= 0:
        raise ValueError("Invalid face box")
    # Preserve the upstream 2.7x context crop and shift it inside image boundaries.
    scale = min(2.7, (width - 1) / w, (height - 1) / h)
    cw, ch = w * scale, h * scale
    left = min(max(0, x + w / 2 - cw / 2), width - 1 - cw)
    top = min(max(0, y + h / 2 - ch / 2), height - 1 - ch)
    patch = frame[int(top) : int(top + ch) + 1, int(left) : int(left + cw) + 1]
    # Upstream functional.to_tensor preserves BGR values 0–255; this export starts with Conv.
    pixels = cv2.resize(patch, (80, 80)).astype(np.float32)
    return np.ascontiguousarray(pixels.transpose(2, 0, 1)[None])


def live_probability(logits):
    values = np.asarray(logits, dtype=np.float32)
    if values.shape != (1, 3) or not np.isfinite(values).all():
        raise ValueError("Invalid PAD output")
    scores = np.exp(values[0] - values.max())
    # Upstream test.py defines class 1 as live; the exporter card's class-0 label conflicts.
    return float(scores[1] / scores.sum())


class LivenessModel:
    def __init__(self, directory, *, threshold=0.95):
        if not math.isfinite(threshold) or not 0 < threshold <= 1:
            raise ValueError("Invalid liveness threshold")
        spec = next(
            s
            for s in json.loads(MANIFEST.read_text())["models"]
            if s["name"] == "face_antispoof_minifasnet"
        )
        path = verify_model(directory, spec)
        options = ort.SessionOptions()
        options.intra_op_num_threads, options.inter_op_num_threads = 2, 1
        self.session = ort.InferenceSession(
            str(path), sess_options=options, providers=["CPUExecutionProvider"]
        )
        self.input = self.session.get_inputs()[0].name
        self.threshold, self.model_sha256 = threshold, spec["sha256"]

    def __call__(self, frame, face):
        logits = self.session.run(None, {self.input: prepare_crop(frame, face)})[0]
        return live_probability(logits) >= self.threshold
