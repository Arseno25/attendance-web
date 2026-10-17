"""Measure real engine latency and fixture-based accuracy; never invent numbers.

Accuracy sections built from synthetic or authorized fixture faces are sanity
checks only. Real-world FPIR/FNIR claims require an authorized hold-out dataset
evaluated through app.evaluation.
"""

import argparse
import json
import math
import os
import platform
import resource
import time
from pathlib import Path

import cv2
import numpy as np

from app.evaluation import summarize
from app.face_engine import EngineConfig, FaceMatcher
from app.pipeline import Pipeline, normalize


def summarize_latencies(values):
    ordered = sorted(float(v) for v in values)
    if not ordered:
        raise ValueError("No latency samples")

    def percentile(fraction):
        return ordered[math.ceil(len(ordered) * fraction) - 1]

    return {
        "count": len(ordered),
        "min": ordered[0],
        "max": ordered[-1],
        "mean": sum(ordered) / len(ordered),
        "p50": percentile(0.5),
        "p95": percentile(0.95),
        "p99": percentile(0.99),
    }


def score_stats(genuine, impostor):
    if not genuine or not impostor:
        raise ValueError("Score sets must be nonempty")

    def part(values):
        scores = [float(v) for v in values]
        return {
            "count": len(scores),
            "min": min(scores),
            "max": max(scores),
            "mean": sum(scores) / len(scores),
        }

    return {"genuine": part(genuine), "impostor": part(impostor)}


def build_probe(expected, predicted, latency_ms, attack):
    if not isinstance(attack, bool):
        raise TypeError("Attack flag must be boolean")
    return {
        "expected": expected,
        "predicted": predicted,
        "latency_ms": float(latency_ms),
        "attack": attack,
    }


def augmentations(image, seed=0):
    if not isinstance(image, np.ndarray) or image.dtype != np.uint8:
        raise ValueError("Augmentation requires a uint8 image")
    rng = np.random.default_rng(seed)
    height, width = image.shape[:2]
    base = image.astype(np.int16)
    items = [
        ("brightness_up", np.clip(base + 25, 0, 255).astype(np.uint8)),
        ("brightness_down", np.clip(base - 25, 0, 255).astype(np.uint8)),
        (
            "contrast",
            np.clip((image.astype(np.float32) - 127.5) * 1.2 + 127.5, 0, 255).astype(
                np.uint8
            ),
        ),
    ]
    encoded = cv2.imencode(".jpg", image, [cv2.IMWRITE_JPEG_QUALITY, 55])[1]
    items.append(("jpeg_recompress", cv2.imdecode(encoded, cv2.IMREAD_COLOR)))
    noise = np.clip(base + rng.normal(0, 12, image.shape), 0, 255).astype(np.uint8)
    items.append(("gaussian_noise", noise))
    items.append(("blur", cv2.GaussianBlur(image, (5, 5), 1.2)))
    small = cv2.resize(image, (max(1, width * 3 // 4), max(1, height * 3 // 4)))
    items.append(("scale_roundtrip", cv2.resize(small, (width, height))))
    return items


def print_replay_approximation(image):
    """Synthetic photo-of-photo approximation; no real print or display hardware."""
    height, width = image.shape[:2]
    small = cv2.resize(image, (max(1, width * 2 // 5), max(1, height * 2 // 5)))
    softened = cv2.GaussianBlur(small, (3, 3), 0.8)
    encoded = cv2.imencode(".jpg", softened, [cv2.IMWRITE_JPEG_QUALITY, 40])[1]
    restored = cv2.imdecode(encoded, cv2.IMREAD_COLOR)
    restored = cv2.resize(restored, (width, height))
    return np.clip(restored.astype(np.int16) + 15, 0, 255).astype(np.uint8)


def cpu_model():
    try:
        for line in Path("/proc/cpuinfo").read_text().splitlines():
            if line.startswith("model name"):
                return line.split(":", 1)[1].strip()
    except OSError:
        pass
    return platform.processor() or "unknown"


def memory_rss_mb():
    return resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024


def timed(callable_, samples, warmup=5):
    for _ in range(warmup):
        callable_()
    durations = []
    for _ in range(samples):
        started = time.perf_counter()
        callable_()
        durations.append((time.perf_counter() - started) * 1000)
    return summarize_latencies(durations)


def detect_primary_face(pipeline, image):
    faces = pipeline.detect(image, threshold=0.6)
    if len(faces) != 1:
        return None
    return faces[0]


def face_centric_frame(image, face, size=(640, 480)):
    width, height = size
    x, y, w, h = (float(value) for value in face[:4])
    margin_x, margin_y = w * 0.35, h * 0.35
    x0, y0 = max(0, int(x - margin_x)), max(0, int(y - margin_y))
    x1 = min(image.shape[1], int(x + w + margin_x))
    y1 = min(image.shape[0], int(y + h + margin_y))
    crop = image[y0:y1, x0:x1]
    if crop.shape[0] > height or crop.shape[1] > width:
        scale = min(width / crop.shape[1], height / crop.shape[0])
        crop = cv2.resize(
            crop,
            (
                max(1, round(crop.shape[1] * scale)),
                max(1, round(crop.shape[0] * scale)),
            ),
        )
    background = np.full((height, width, 3), 40, dtype=np.uint8)
    top = (height - crop.shape[0]) // 2
    left = (width - crop.shape[1]) // 2
    background[top : top + crop.shape[0], left : left + crop.shape[1]] = crop
    return background


def random_gallery(model_sha256, identities, templates=2, seed=1234):
    rng = np.random.default_rng(seed)
    return {
        f"student-{index}": [
            normalize(rng.normal(0, 1, 128).astype(np.float32))
            for _ in range(templates)
        ]
        for index in range(identities)
    }


def load_fixtures(faces_dir):
    if faces_dir is None:
        return []
    directory = Path(faces_dir)
    if not directory.is_dir():
        raise ValueError("Faces directory not found")
    items = []
    for path in sorted(directory.iterdir()):
        if path.suffix.lower() not in {".jpg", ".jpeg", ".png", ".bmp", ".webp"}:
            continue
        image = cv2.imread(str(path))
        if image is not None:
            items.append((path.stem, image))
    return items


def pad_probability(pad_model, frame, face):
    from app.liveness import live_probability, prepare_crop

    logits = pad_model.session.run(None, {pad_model.input: prepare_crop(frame, face)})[
        0
    ]
    return live_probability(logits)


def run_accuracy(pipeline, config, fixtures, pad_model=None):
    detected = []
    for name, image in fixtures:
        face = detect_primary_face(pipeline, image)
        if face is not None:
            detected.append((name, image, face))
    if len(detected) < 2:
        return {
            "status": "NOT VALIDATED",
            "reason": "Fewer than two fixture images contain one detectable face",
            "available_faces": len(detected),
        }
    identities = {}
    for name, image, face in detected:
        identities[name] = {
            "image": image,
            "face": face,
            "embedding": pipeline.feature(image, face),
        }
    margin = config.margin
    genuine, impostor = [], []
    probes = []
    known_rows, unknown_rows, attack_rows, pad_rows = [], [], [], []

    def record_match(embedding, gallery_ids):
        matcher = FaceMatcher(pipeline.model_sha256, config)
        matcher.load_embeddings(
            {key: [identities[key]["embedding"]] for key in gallery_ids},
            model_sha256=pipeline.model_sha256,
        )
        match = matcher.match(embedding)
        return {
            "subject": match.student_id,
            "best": match.similarity,
            "margin": match.margin,
            "runner_up": (
                match.candidates[1].similarity if len(match.candidates) > 1 else -1.0
            ),
        }

    for name in identities:
        started = time.perf_counter()
        unknown = record_match(
            identities[name]["embedding"],
            [key for key in identities if key != name],
        )
        elapsed = (time.perf_counter() - started) * 1000
        if unknown["best"] is not None:
            impostor.append(unknown["best"])
        unknown_rows.append(unknown)
        probes.append(build_probe(None, unknown["subject"], elapsed, False))
    for name, entry in identities.items():
        matcher = FaceMatcher(pipeline.model_sha256, config)
        matcher.load_embeddings(
            {key: [identities[key]["embedding"]] for key in identities},
            model_sha256=pipeline.model_sha256,
        )
        for variant, image in augmentations(entry["image"], seed=len(name)):
            started = time.perf_counter()
            face = detect_primary_face(pipeline, image)
            if face is None:
                row = {"expected": name, "subject": None, "best": None, "margin": None}
            else:
                match = matcher.match(pipeline.feature(image, face))
                row = {
                    "expected": name,
                    "subject": match.student_id,
                    "best": match.similarity,
                    "margin": match.margin,
                }
                if match.similarity is not None:
                    genuine.append(match.similarity)
                    runner = (
                        match.candidates[1].similarity
                        if len(match.candidates) > 1
                        else -1.0
                    )
                    if runner >= -1.0:
                        impostor.append(runner)
            elapsed = (time.perf_counter() - started) * 1000
            known_rows.append(row)
            probes.append(build_probe(name, row["subject"], elapsed, False))
        started = time.perf_counter()
        attack_image = print_replay_approximation(entry["image"])
        face = detect_primary_face(pipeline, attack_image)
        if face is None:
            attack = {"subject": None, "best": None, "margin": None}
        else:
            match = matcher.match(pipeline.feature(attack_image, face))
            attack = {
                "subject": match.student_id,
                "best": match.similarity,
                "margin": match.margin,
            }
        elapsed = (time.perf_counter() - started) * 1000
        attack_rows.append(attack)
        probes.append(build_probe(None, attack["subject"], elapsed, True))
        if pad_model is not None:
            base_face = entry["face"]
            attack_face = face if face is not None else base_face
            pad_rows.append(
                {
                    "identity": name,
                    "live_probability_clean": pad_probability(
                        pad_model, entry["image"], base_face
                    ),
                    "live_probability_print_replay_approx": pad_probability(
                        pad_model, attack_image, attack_face
                    ),
                }
            )

    def accepted(row, level):
        return (
            row["subject"] is not None
            and row["best"] is not None
            and row["best"] >= level
            and row["margin"] is not None
            and row["margin"] >= margin
        )

    sweep = []
    for step in range(6, 19):
        level = step / 20
        sweep.append(
            {
                "threshold": level,
                "fnir": sum(
                    1
                    for row in known_rows
                    if not accepted(row, level) or row["subject"] != row["expected"]
                )
                / max(1, len(known_rows)),
                "fpir": sum(1 for row in unknown_rows if accepted(row, level))
                / max(1, len(unknown_rows)),
                "attack_pass_rate": sum(
                    1 for row in attack_rows if accepted(row, level)
                )
                / max(1, len(attack_rows)),
            }
        )
    return {
        "status": "FIXTURE SANITY ONLY",
        "note": (
            "Synthetic/authorized fixture faces; NOT VALIDATED for real-world "
            "FPIR/FNIR. Run app.evaluation on an authorized hold-out dataset "
            "before approving production attendance."
        ),
        "identities": len(identities),
        "configured_threshold": config.threshold,
        "configured_margin": margin,
        "detection_threshold_used": 0.6,
        "official_summary": summarize(probes),
        "pad_probabilities": pad_rows or None,
        "score_stats": score_stats(genuine, impostor) if genuine and impostor else None,
        "threshold_sweep_margin_fixed": sweep,
    }


def main():
    parser = argparse.ArgumentParser(
        description="Benchmark the real YuNet/SFace pipeline; no fabricated numbers."
    )
    parser.add_argument("--directory", required=True, help="Verified models directory")
    parser.add_argument("--faces-dir", help="Optional fixture face images")
    parser.add_argument("--gallery", type=int, default=500)
    parser.add_argument("--iterations", type=int, default=150)
    parser.add_argument("--output", help="Write JSON results to this path")
    args = parser.parse_args()
    if not 1 <= args.gallery <= 3000 or not 10 <= args.iterations <= 5000:
        raise SystemExit("Invalid benchmark bounds")

    started = time.perf_counter()
    pipeline = Pipeline(args.directory)
    config = EngineConfig(
        threshold=float(os.getenv("AI_MATCH_THRESHOLD", "0.8")),
        margin=float(os.getenv("AI_MATCH_MARGIN", "0.1")),
    )
    load_seconds = time.perf_counter() - started
    fixtures = load_fixtures(args.faces_dir)
    face_frame = None
    primary_face = None
    for _, image in fixtures:
        source_face = detect_primary_face(pipeline, image)
        if source_face is None:
            continue
        candidate = face_centric_frame(image, source_face)
        composed_face = detect_primary_face(pipeline, candidate)
        if composed_face is not None:
            face_frame, primary_face = candidate, composed_face
            break
    noise = (
        np.random.default_rng(99)
        .integers(0, 256, (480, 640, 3), dtype=np.uint8)
        .astype(np.uint8)
    )

    detection = {}
    for width, height in ((640, 480), (960, 720), (1280, 720)):
        frame = cv2.resize(
            face_frame if face_frame is not None else noise, (width, height)
        )
        detection[f"{width}x{height}"] = timed(
            lambda frame=frame: pipeline.detect(frame, threshold=0.9), args.iterations
        )
    if primary_face is not None:
        embedding = timed(
            lambda: pipeline.feature(face_frame, primary_face), args.iterations
        )
        pad_source = (face_frame, primary_face)
    else:
        embedding = None
        synthetic_row = np.array(
            [120.0, 80.0, 100.0, 80.0, 160.0, 80.0, 140.0, 140.0, 130.0],
            dtype=np.float32,
        )
        pad_source = (noise, synthetic_row)

    def load_matcher(identities):
        matcher = FaceMatcher(pipeline.model_sha256, config)
        matcher.load_embeddings(
            random_gallery(pipeline.model_sha256, identities),
            model_sha256=pipeline.model_sha256,
        )
        return matcher

    probe_vector = normalize(
        np.random.default_rng(5).normal(0, 1, 128).astype(np.float32)
    )
    matching = {}
    for identities in sorted({100, 500, args.gallery}):
        matcher = load_matcher(identities)
        matching[f"{identities}_identities_x2"] = timed(
            lambda matcher=matcher: matcher.match(probe_vector), args.iterations
        )

    from app.liveness import LivenessModel

    pad_model = LivenessModel(args.directory)
    pad = timed(lambda: pad_model(pad_source[0], pad_source[1]), args.iterations)

    from app.face_engine import FaceRecognitionEngine

    engine = FaceRecognitionEngine(config, pipeline=pipeline, liveness_hook=pad_model)
    engine.load_embeddings(
        random_gallery(pipeline.model_sha256, args.gallery),
        model_sha256=pipeline.model_sha256,
    )
    statuses = {}
    end_to_end_samples = []
    e2e_frame = face_frame if face_frame is not None else noise
    for index in range(args.iterations):
        at = time.perf_counter()
        result = engine.process_frame(e2e_frame, f"bench-{index}")
        end_to_end_samples.append((time.perf_counter() - at) * 1000)
        statuses[result.status] = statuses.get(result.status, 0) + 1
    engine_names = sorted(statuses.items(), key=lambda item: -item[1])
    throughput = {
        "frames_per_second": round(
            args.iterations / (sum(end_to_end_samples) / 1000), 2
        ),
        "designed_session_cadence_per_second": 1 / config.min_interval,
        "note": "One inference at a time; per-session embedding capped by min_interval",
    }

    accuracy = run_accuracy(pipeline, config, fixtures, pad_model)
    report = {
        "environment": {
            "cpu": cpu_model(),
            "logical_cores": os.cpu_count(),
            "python": platform.python_version(),
            "opencv": cv2.__version__,
            "threads_env": os.getenv("AI_OPENCV_THREADS", "2"),
            "max_rss_mb": round(memory_rss_mb(), 1),
            "model_load_seconds": round(load_seconds, 3),
        },
        "models": {
            "detector_sha256": pipeline.detector_sha256,
            "recognition_sha256": pipeline.model_sha256,
            "pad_sha256": pad_model.model_sha256,
        },
        "latency_ms": {
            "detection": detection,
            "embedding": embedding,
            "matching": matching,
            "pad": pad,
            "end_to_end_process_frame": summarize_latencies(end_to_end_samples),
        },
        "throughput": throughput,
        "end_to_end_statuses": dict(engine_names),
        "accuracy": accuracy,
    }
    rendered = json.dumps(report, indent=2)
    if args.output:
        Path(args.output).write_text(rendered + "\n")
    print(rendered)


if __name__ == "__main__":
    main()
