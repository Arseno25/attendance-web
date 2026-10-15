"""Internal 1:N recognition only. Similarities are not probabilities or liveness."""

import math
import threading
import time
from collections import OrderedDict, deque
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass, field
from typing import Literal

import cv2
import numpy as np

from app.pipeline import ImageError, Pipeline, get_pipeline, normalize

Status = Literal[
    "NO_FACE",
    "MULTIPLE_FACES",
    "LOW_QUALITY",
    "PROCESSING",
    "CONFIRMED",
    "VERIFYING",
    "UNKNOWN",
    "ERROR",
]


@dataclass(frozen=True)
class EngineConfig:
    threshold: float
    margin: float
    detection_threshold: float = 0.85
    confirmation_window: int = 5
    consistent_matches: int = 3
    min_face_size: int = 100
    min_interval: float = 0.2
    session_timeout: float = 3.0
    max_frame_age: float = 2.0
    max_sessions: int = 128
    top_k: int = 3

    def __post_init__(self):
        numbers = (
            self.threshold,
            self.detection_threshold,
            self.margin,
            self.min_interval,
            self.session_timeout,
            self.max_frame_age,
        )
        if (
            not all(math.isfinite(v) for v in numbers)
            or not 0 < self.detection_threshold <= 1
            or not -1 <= self.threshold <= 1
            or not 0 < self.margin <= 2
        ):
            raise ValueError("Invalid recognition thresholds")
        if any(
            type(v) is not int
            for v in (
                self.confirmation_window,
                self.consistent_matches,
                self.min_face_size,
                self.max_sessions,
                self.top_k,
            )
        ):
            raise ValueError("Recognition counts must be integers")
        if (
            not 2 <= self.consistent_matches <= self.confirmation_window <= 32
            or not 1 <= self.max_sessions <= 1024
            or not 1 <= self.top_k <= 20
        ):
            raise ValueError("Invalid recognition bounds")
        if (
            self.min_face_size < 80
            or self.min_interval < 0
            or self.session_timeout <= 0
            or self.max_frame_age <= 0
        ):
            raise ValueError("Invalid recognition timing or quality")


@dataclass(frozen=True)
class Candidate:
    student_id: str
    similarity: float


@dataclass(frozen=True)
class Match:
    student_id: str | None
    similarity: float | None
    margin: float | None
    candidates: tuple[Candidate, ...] = ()


@dataclass(frozen=True)
class RecognitionResult:
    status: Status
    student_id: str | None = None
    similarity: float | None = None
    face_detected: bool = False
    face_count: int = 0
    faces: tuple[dict, ...] = ()
    is_confirmed: bool = False
    recognition_only: bool = True
    liveness_passed: bool | None = None
    reason: str | None = None
    timings_ms: dict[str, float] = field(default_factory=dict)


class FaceDetector:
    """Share the checksum-verified Pipeline's single mutable YuNet instance."""

    def __init__(self, pipeline: Pipeline, threshold: float = 0.85):
        self.pipeline, self.threshold = pipeline, threshold

    def detect(self, frame: np.ndarray) -> np.ndarray:
        scale = min(640 / frame.shape[1], 480 / frame.shape[0], 1.0)
        resized = (
            cv2.resize(
                frame, (round(frame.shape[1] * scale), round(frame.shape[0] * scale))
            )
            if scale < 1
            else frame
        )
        faces = self.pipeline.detect(resized, threshold=self.threshold)
        if faces.ndim != 2 or faces.shape[1] != 15 or not np.isfinite(faces).all():
            raise ValueError("Invalid detector output")
        if scale < 1:
            faces[:, :14] /= scale
        return faces[faces[:, 14] >= self.threshold]


class FaceRecognizer:
    def __init__(self, pipeline: Pipeline):
        self.pipeline = pipeline

    def extract_embedding(self, frame: np.ndarray, detection: np.ndarray) -> np.ndarray:
        return normalize(self.pipeline.feature(frame, detection))


class FaceMatcher:
    """Max template score per identity; runner-up is always a different identity."""

    def __init__(self, model_sha256: str, config: EngineConfig):
        if len(model_sha256) != 64 or any(
            c not in "0123456789abcdef" for c in model_sha256
        ):
            raise ValueError("Invalid model SHA256")
        self.model_sha256, self.config = model_sha256, config
        self._lock = threading.RLock()
        self._templates: dict[str, np.ndarray] = {}
        self._matrix = np.empty((0, 128), np.float32)
        self._subjects: tuple[str, ...] = ()
        self._starts = np.empty(0, np.intp)

    def _validate(self, gallery, model_sha256):
        if model_sha256 != self.model_sha256:
            raise ValueError("Incompatible embedding model")
        if len(gallery) > 3000:
            raise ValueError("Gallery exceeds 3000 identities")
        templates = {}
        count = 0
        for subject, vectors in gallery.items():
            if not isinstance(subject, str) or not subject or len(subject) > 128:
                raise ValueError("Invalid student identity")
            rows = np.asarray(vectors, dtype=np.float32)
            if rows.ndim != 2 or rows.shape[1] != 128 or not 1 <= len(rows) <= 5:
                raise ValueError("Require 1–5 compatible templates per identity")
            templates[subject] = np.stack([normalize(row) for row in rows])
            count += len(rows)
        if count > 10000:
            raise ValueError("Gallery exceeds 10000 templates")
        return templates

    def _replace(self, templates):
        subjects = tuple(templates)
        matrix = (
            np.concatenate(tuple(templates.values()))
            if templates
            else np.empty((0, 128), np.float32)
        )
        starts = (
            np.cumsum([0] + [len(templates[s]) for s in subjects[:-1]], dtype=np.intp)
            if subjects
            else np.empty(0, np.intp)
        )
        self._templates, self._subjects, self._matrix, self._starts = (
            templates,
            subjects,
            matrix,
            starts,
        )

    def load_embeddings(
        self, gallery: Mapping[str, Sequence[np.ndarray]], *, model_sha256: str
    ):
        templates = self._validate(gallery, model_sha256)
        with self._lock:
            self._replace(templates)

    def update_student_embeddings(
        self, student_id: str, vectors: Sequence[np.ndarray], *, model_sha256: str
    ):
        update = self._validate({student_id: vectors}, model_sha256)
        with self._lock:
            templates = self._templates | update
            if len(templates) > 3000 or sum(map(len, templates.values())) > 10000:
                raise ValueError("Gallery capacity exceeded")
            self._replace(templates)

    def remove_student(self, student_id: str):
        with self._lock:
            self._replace({s: v for s, v in self._templates.items() if s != student_id})

    def match(self, embedding: np.ndarray) -> Match:
        if np.asarray(embedding).shape != (128,):
            raise ValueError("Invalid probe dimensions")
        probe = normalize(embedding)
        with self._lock:
            if not self._subjects:
                return Match(None, None, None)
            scores = np.maximum.reduceat(self._matrix @ probe, self._starts)
            order = np.argsort(-scores, kind="stable")[: max(2, self.config.top_k)]
            best = float(np.clip(scores[order[0]], -1, 1))
            runner_up = (
                float(np.clip(scores[order[1]], -1, 1)) if len(order) > 1 else -1.0
            )
            margin = best - runner_up
            identity = (
                self._subjects[order[0]]
                if best >= self.config.threshold and margin >= self.config.margin
                else None
            )
            candidates = tuple(
                Candidate(self._subjects[i], float(np.clip(scores[i], -1, 1)))
                for i in order[: self.config.top_k]
            )
            return Match(identity, best, margin, candidates)


class RecognitionSession:
    def __init__(self, config: EngineConfig):
        self.config = config
        self.matches: deque[str] = deque(maxlen=config.confirmation_window)
        self.state = "WAITING"
        self.last_timestamp = -math.inf
        self.last_feature_at = -math.inf
        self.last_seen = -math.inf
        self.previous_embedding: np.ndarray | None = None
        self.box: np.ndarray | None = None
        self.identity: str | None = None
        self.verified_identity: str | None = None

    def reset(self):
        # Retain timestamp high-water mark to reject replay after disappearance.
        self.matches.clear()
        self.state = "RESET"
        self.previous_embedding = self.box = self.identity = self.verified_identity = (
            None
        )
        self.last_feature_at = -math.inf

    def observe(self, match: Match, embedding: np.ndarray, timestamp: float) -> bool:
        if (
            self.previous_embedding is not None
            and float(embedding @ self.previous_embedding) < 0.6
        ):
            # shortcut: provisional continuity cutoff, calibrate with authorized tracking data.
            self.reset()
        if match.student_id != self.identity:
            self.reset()
        self.previous_embedding = embedding
        self.identity = match.student_id
        self.last_feature_at = timestamp
        if match.student_id is None:
            self.state = "UNKNOWN"
            return False
        self.matches.append(match.student_id)
        confirmed = len(self.matches) >= self.config.consistent_matches
        self.state = "CONFIRMED" if confirmed else "RECOGNIZING"
        return confirmed


def _overlap(a, b):
    left, top = np.maximum(a[:2], b[:2])
    right, bottom = np.minimum(a[:2] + a[2:4], b[:2] + b[2:4])
    area = max(0, right - left) * max(0, bottom - top)
    union = a[2] * a[3] + b[2] * b[3] - area
    return float(area / union) if union > 0 else 0.0


class FaceRecognitionEngine:
    """Trusted internal caller supplies authorized embeddings and monotonic timestamps.

    One inference at a time; excess requests return ERROR/INFERENCE_BUSY. A separate
    PAD hook reports liveness evidence only; it never authorizes attendance.
    """

    def __init__(
        self,
        config: EngineConfig,
        *,
        pipeline: Pipeline | None = None,
        liveness_hook: Callable[[np.ndarray, np.ndarray], bool | None] | None = None,
    ):
        self.config = config
        self.pipeline = pipeline if pipeline is not None else get_pipeline()
        self.detector = FaceDetector(self.pipeline, config.detection_threshold)
        self.recognizer = FaceRecognizer(self.pipeline)
        self.matcher = FaceMatcher(self.pipeline.model_sha256, config)
        self.liveness_hook = liveness_hook
        self._sessions: OrderedDict[str, RecognitionSession] = OrderedDict()
        self._lock = threading.RLock()
        self._slot = threading.BoundedSemaphore(1)

    def load_embeddings(self, gallery, *, model_sha256: str):
        with self._lock:
            self.matcher.load_embeddings(gallery, model_sha256=model_sha256)
            self._sessions.clear()

    def load_encrypted_templates(self, templates, cipher, *, school_id: str):
        """School-bound AES-GCM records; legacy consent IDs remain encryption context."""
        gallery = {}
        for template in templates:
            if template["model_sha256"] != self.pipeline.model_sha256:
                raise ValueError("Incompatible embedding model")
            subject = template["student_id"]
            vector = cipher.decrypt(
                template, school_id, subject, template.get("consent_id")
            )
            gallery.setdefault(subject, []).append(vector)
        self.load_embeddings(gallery, model_sha256=self.pipeline.model_sha256)

    def update_student_embeddings(self, student_id, vectors, *, model_sha256: str):
        with self._lock:
            self.matcher.update_student_embeddings(
                student_id, vectors, model_sha256=model_sha256
            )
            self._sessions.clear()

    def remove_student(self, student_id: str):
        with self._lock:
            self.matcher.remove_student(student_id)
            self._sessions.clear()

    def reset_session(self, session_id: str):
        with self._lock:
            if session_id in self._sessions:
                self._sessions[session_id].reset()

    def process_frame(
        self, frame: np.ndarray, session_id: str, *, timestamp: float | None = None
    ) -> RecognitionResult:
        now = time.monotonic()
        timestamp = now if timestamp is None else timestamp
        if not isinstance(session_id, str) or not session_id or len(session_id) > 128:
            return RecognitionResult("ERROR", reason="INVALID_SESSION")
        if (
            not isinstance(timestamp, (int, float))
            or not math.isfinite(timestamp)
            or now - timestamp > self.config.max_frame_age
            or timestamp - now > 0.5
        ):
            self.reset_session(session_id)
            return RecognitionResult("ERROR", reason="STALE_FRAME")
        if (
            not isinstance(frame, np.ndarray)
            or frame.dtype != np.uint8
            or frame.ndim != 3
            or frame.shape[2] != 3
            or not (160 <= frame.shape[0] <= 1280 and 160 <= frame.shape[1] <= 1280)
        ):
            self.reset_session(session_id)
            return RecognitionResult("ERROR", reason="INVALID_FRAME")
        if not self._slot.acquire(blocking=False):
            return RecognitionResult("ERROR", reason="INFERENCE_BUSY")
        started = time.perf_counter()
        timings = {}
        try:
            with self._lock:
                expired = [
                    key
                    for key, value in self._sessions.items()
                    if now - value.last_seen > self.config.session_timeout
                ]
                for key in expired:
                    del self._sessions[key]
                session = self._sessions.setdefault(
                    session_id, RecognitionSession(self.config)
                )
                self._sessions.move_to_end(session_id)
                while len(self._sessions) > self.config.max_sessions:
                    self._sessions.popitem(last=False)
                if timestamp <= session.last_timestamp:
                    session.reset()
                    return RecognitionResult("ERROR", reason="STALE_FRAME")
                session.last_timestamp, session.last_seen = timestamp, now
                session.state = "DETECTING"
                at = time.perf_counter()
                faces = self.detector.detect(frame)
                boxes = tuple(self.pipeline.boxes(frame, faces))
                timings["detection"] = (time.perf_counter() - at) * 1000
                count = len(faces)
                if count != 1:
                    session.reset()
                    return RecognitionResult(
                        "NO_FACE" if count == 0 else "MULTIPLE_FACES",
                        face_detected=count > 0,
                        face_count=count,
                        faces=boxes,
                        timings_ms=timings,
                    )
                try:
                    self.pipeline.check_quality(
                        frame, faces[0], min_size=self.config.min_face_size
                    )
                except ImageError:
                    session.reset()
                    return RecognitionResult(
                        "LOW_QUALITY",
                        face_detected=True,
                        face_count=1,
                        faces=boxes,
                        timings_ms=timings,
                    )
                box = faces[0][:4]
                if session.box is not None and _overlap(box, session.box) < 0.3:
                    session.reset()
                session.box = box.copy()
                if timestamp - session.last_feature_at < self.config.min_interval:
                    return RecognitionResult(
                        "PROCESSING",
                        face_detected=True,
                        face_count=1,
                        reason="INFERENCE_INTERVAL",
                        faces=boxes,
                        timings_ms=timings,
                    )
                at = time.perf_counter()
                embedding = self.recognizer.extract_embedding(frame, faces[0])
                timings["embedding"] = (time.perf_counter() - at) * 1000
                at = time.perf_counter()
                match = self.matcher.match(embedding)
                timings["matching"] = (time.perf_counter() - at) * 1000
                confirmed = session.observe(match, embedding, timestamp)
                try:
                    live = (
                        self.liveness_hook(frame, faces[0])
                        if self.liveness_hook
                        else None
                    )
                    if live is not None and not isinstance(live, bool):
                        raise ValueError("Invalid PAD result")
                except Exception:  # noqa: BLE001 — external PAD failures must clear identity state.
                    session.reset()
                    return RecognitionResult("ERROR", reason="PAD_FAILED")
                if live is False:
                    if (
                        match.student_id is not None
                        and session.verified_identity == match.student_id
                    ):
                        return RecognitionResult(
                            "VERIFYING",
                            student_id=match.student_id,
                            similarity=match.similarity,
                            face_detected=True,
                            face_count=1,
                            faces=boxes,
                            is_confirmed=True,
                            liveness_passed=False,
                            reason="PAD_REJECTED",
                            timings_ms=timings,
                        )
                    session.reset()
                    return RecognitionResult(
                        "UNKNOWN",
                        face_detected=True,
                        face_count=1,
                        liveness_passed=False,
                        reason="PAD_REJECTED",
                        faces=boxes,
                        timings_ms=timings,
                    )
                if confirmed and live is True:
                    session.verified_identity = match.student_id
                timings["total"] = (time.perf_counter() - started) * 1000
                result_status = (
                    "CONFIRMED"
                    if confirmed
                    else "PROCESSING"
                    if match.student_id is not None
                    else "UNKNOWN"
                )
                return RecognitionResult(
                    result_status,
                    student_id=match.student_id if confirmed else None,
                    similarity=match.similarity,
                    face_detected=True,
                    face_count=1,
                    faces=boxes,
                    is_confirmed=confirmed,
                    liveness_passed=live,
                    timings_ms=timings,
                )
        except cv2.error, ValueError, OSError, RuntimeError, TypeError, IndexError:
            self.reset_session(session_id)
            return RecognitionResult("ERROR", reason="INFERENCE_FAILED")
        finally:
            self._slot.release()
