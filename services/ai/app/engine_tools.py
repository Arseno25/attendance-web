"""Anonymous authorized-data evaluation and explicitly synthetic CPU benchmarks."""

import argparse
import base64
import hashlib
import json
import re
import resource
import time
from dataclasses import replace
from pathlib import Path

import numpy as np

from app.evaluation import summarize
from app.face_engine import EngineConfig, FaceMatcher, FaceRecognitionEngine
from app.pipeline import Pipeline, decode_image


def distribution(values):
    return (
        {
            "samples": len(values),
            "mean_ms": float(np.mean(values)),
            "p95_ms": float(np.percentile(values, 95)),
        }
        if values
        else None
    )


def benchmark(directory, *, identities=1000, iterations=30):
    if not 1 <= identities <= 3000 or not 1 <= iterations <= 1000:
        raise ValueError("Invalid benchmark bounds")
    config = EngineConfig(threshold=0.8, margin=0.1, min_interval=0, cooldown=0)
    pipeline = Pipeline(directory)
    matcher = FaceMatcher(pipeline.model_sha256, config)
    rng = np.random.default_rng(17)
    matcher.load_embeddings(
        {
            f"anon-{i}": rng.standard_normal((3, 128), dtype=np.float32)
            for i in range(identities)
        },
        model_sha256=pipeline.model_sha256,
    )
    probe = rng.standard_normal(128, dtype=np.float32)
    blank = np.zeros((480, 640, 3), np.uint8)
    noise = rng.integers(0, 256, (320, 320, 3), dtype=np.uint8)
    face = np.array(
        [70, 70, 150, 180, 110, 125, 180, 125, 145, 160, 120, 200, 175, 200, 0.99],
        np.float32,
    )
    # Warm allocation without recording model initialization in inference timing.
    pipeline.detect(blank)
    pipeline.feature(noise, face)
    matcher.match(probe)
    wall, cpu = time.perf_counter(), time.process_time()
    stages = {name: [] for name in ("detection", "embedding", "matching", "total")}
    for _ in range(iterations):
        started = time.perf_counter()
        at = time.perf_counter()
        pipeline.detect(blank)
        stages["detection"].append((time.perf_counter() - at) * 1000)
        at = time.perf_counter()
        pipeline.feature(noise, face)
        stages["embedding"].append((time.perf_counter() - at) * 1000)
        at = time.perf_counter()
        matcher.match(probe)
        stages["matching"].append((time.perf_counter() - at) * 1000)
        stages["total"].append((time.perf_counter() - started) * 1000)
    elapsed, used = time.perf_counter() - wall, time.process_time() - cpu
    return {
        "kind": "synthetic_cpu_smoke_not_face_accuracy",
        "gallery_identities": identities,
        "templates_per_identity": 3,
        "model_sha256": pipeline.model_sha256,
        "timings": {name: distribution(values) for name, values in stages.items()},
        "cpu_process_percent": used / elapsed * 100,
        "max_rss_mib": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss / 1024,
        "notes": "Detection: blank 640x480. Embedding: noise with synthetic landmarks. Total: summed artificial stages, not live identification FPS. RSS includes models/runtime; CPU may exceed 100% with multiple threads.",
        "automatic_attendance_approved": False,
    }


def evaluate(
    dataset: Path,
    directory: Path,
    config: EngineConfig,
    *,
    authorized: bool,
    calibration: bool = False,
):
    if not authorized:
        raise ValueError("An explicitly authorized dataset is required")
    data = json.loads(dataset.read_text())
    expected_split = "calibration" if calibration else "holdout"
    if data.get("split") != expected_split:
        raise ValueError("Use an independent dataset with the requested split")
    pipeline = Pipeline(directory)
    if data.get("model_sha256") != pipeline.model_sha256:
        raise ValueError("Incompatible dataset model")
    anonymous = re.compile(r"anon-[a-z0-9_-]{1,64}\Z")
    gallery, gallery_digests = {}, set()

    def read_frame(relative):
        if not isinstance(relative, str):
            raise TypeError("Invalid dataset frame path")
        raw = (dataset.parent / relative).read_bytes()
        frame, _ = decode_image(base64.b64encode(raw).decode())
        return frame, hashlib.sha256(raw).digest()

    if not data.get("gallery") or len(data["gallery"]) > 3000:
        raise ValueError("Invalid evaluation gallery")
    for subject, paths in data["gallery"].items():
        if not anonymous.fullmatch(subject) or not 1 <= len(paths) <= 5:
            raise ValueError("Use anonymous gallery IDs and 1–5 frames")
        vectors = []
        for relative in paths:
            frame, digest = read_frame(relative)
            if digest in gallery_digests:
                raise ValueError("Duplicate gallery image")
            gallery_digests.add(digest)
            faces = pipeline.detect(frame)
            if len(faces) != 1:
                raise ValueError("Gallery image needs exactly one face")
            pipeline.check_quality(frame, faces[0], min_size=config.min_face_size)
            vectors.append(pipeline.feature(frame, faces[0]))
        gallery[subject] = vectors
    prepared = []
    for row in data.get("probes", []):
        if row.get("attack", False) is not False:
            raise ValueError(
                "Presentation attacks require a separate validated PAD evaluation"
            )
        expected = row["expected"]
        if expected is not None and (
            not isinstance(expected, str)
            or not anonymous.fullmatch(expected)
            or expected not in gallery
        ):
            raise ValueError("Use anonymous enrolled IDs or null for unknown probes")
        if not 3 <= len(row["frames"]) <= 32:
            raise ValueError("Each hold-out track requires 3–32 distinct frames")
        seen = set()
        for relative in row["frames"]:
            frame, digest = read_frame(relative)
            if digest in gallery_digests or digest in seen:
                raise ValueError("Hold-out frame overlaps gallery or repeats")
            seen.add(digest)
        prepared.append((expected, row["frames"]))
    if not prepared:
        raise ValueError("No evaluation probes")
    # Offline supplied frames bypass sampling delay; temporal confirmation remains enabled.
    config = replace(config, min_interval=0, cooldown=0)
    settings = [(config.threshold, config.margin)]
    if calibration:
        settings = [
            (threshold, margin)
            for threshold in (0.4, 0.5, 0.6, 0.7, 0.8, 0.9)
            for margin in (0.05, 0.1, 0.15)
        ]
    reports = []
    for threshold, margin in settings:
        engine = FaceRecognitionEngine(
            replace(config, threshold=threshold, margin=margin), pipeline=pipeline
        )
        engine.load_embeddings(gallery, model_sha256=pipeline.model_sha256)
        traces, stages = (
            [],
            {name: [] for name in ("detection", "embedding", "matching", "total")},
        )
        wall, cpu = time.perf_counter(), time.process_time()
        for index, (expected, paths) in enumerate(prepared):
            predictions = set()
            started = time.perf_counter()
            for relative in paths:
                frame, _ = read_frame(relative)
                result = engine.process_frame(frame, f"probe-{index}")
                if result.status == "ERROR":
                    raise ValueError("Evaluation inference failed")
                if result.is_confirmed:
                    predictions.add(result.student_id)
                for name, elapsed in result.timings_ms.items():
                    stages[name].append(elapsed)
            if len(predictions) > 1:
                raise ValueError(
                    "Probe track contains unstable identities; inspect authorized dataset"
                )
            traces.append(
                {
                    "expected": expected,
                    "predicted": next(iter(predictions), None),
                    "latency_ms": (time.perf_counter() - started) * 1000,
                    "attack": False,
                }
            )
        elapsed, used = time.perf_counter() - wall, time.process_time() - cpu
        reports.append(
            {
                "threshold": threshold,
                "margin": margin,
                **summarize(traces),
                "unknown_rejection_rate": None
                if not any(r["expected"] is None for r in traces)
                else sum(
                    r["expected"] is None and r["predicted"] is None for r in traces
                )
                / sum(r["expected"] is None for r in traces),
                "timings": {
                    name: distribution(values) for name, values in stages.items()
                },
                "cpu_process_percent": used / elapsed * 100,
                "max_rss_mib": resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
                / 1024,
            }
        )
    return {
        "kind": expected_split,
        "model_sha256": pipeline.model_sha256,
        "reports": reports,
        "recognition_only": True,
        "notes": "Calibration sweep is exploratory, not a deployment threshold recommendation. Validate chosen settings on separate representative hold-out identities/cameras. This dataset evaluates recognition only; no PAD/attack evidence is inferred.",
        "automatic_attendance_approved": False,
    }


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mode", choices=("benchmark", "evaluate", "calibrate"))
    parser.add_argument("--models", required=True, type=Path)
    parser.add_argument("--dataset", type=Path)
    parser.add_argument("--authorized-dataset", action="store_true")
    parser.add_argument("--threshold", type=float)
    parser.add_argument("--margin", type=float)
    parser.add_argument("--identities", type=int, default=1000)
    parser.add_argument("--iterations", type=int, default=30)
    args = parser.parse_args()
    if args.mode == "benchmark":
        report = benchmark(
            args.models, identities=args.identities, iterations=args.iterations
        )
    else:
        if args.dataset is None or args.threshold is None or args.margin is None:
            parser.error(
                "dataset, threshold and margin are required for authorized evaluation/calibration"
            )
        report = evaluate(
            args.dataset,
            args.models,
            EngineConfig(args.threshold, args.margin),
            authorized=args.authorized_dataset,
            calibration=args.mode == "calibrate",
        )
    print(json.dumps(report, indent=2))


if __name__ == "__main__":
    main()
