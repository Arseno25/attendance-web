"""Benchmark helpers are deterministic; model accuracy needs authorized data."""

import numpy as np

from app.evaluation import summarize


def test_latency_summary_uses_nearest_rank_percentiles_like_evaluation():
    from app.benchmark import summarize_latencies

    values = [float(v) for v in range(1, 11)]
    stats = summarize_latencies(values)
    assert stats["count"] == 10
    assert stats["p50"] == 5
    assert stats["p95"] == 10
    assert stats["p99"] == 10
    assert stats["min"] == 1
    assert stats["max"] == 10
    assert stats["mean"] == 5.5
    try:
        summarize_latencies([])
    except ValueError:
        pass
    else:
        raise AssertionError("empty samples must fail")


def test_augmentations_are_deterministic_bounded_and_labeled():
    from app.benchmark import augmentations

    image = np.full((64, 64, 3), 120, dtype=np.uint8)
    first = augmentations(image, seed=7)
    second = augmentations(image, seed=7)
    assert [name for name, _ in first] == [name for name, _ in second]
    assert len(first) >= 5
    for (name_a, left), (name_b, right) in zip(first, second, strict=True):
        assert name_a == name_b
        assert np.array_equal(left, right)
        assert left.dtype == np.uint8
        assert left.shape == image.shape
    assert len({name for name, _ in first}) == len(first)


def test_score_stats_separate_genuine_and_impostor_distributions():
    from app.benchmark import score_stats

    stats = score_stats([0.75, 0.82, 0.9], [0.1, 0.2, 0.3])
    assert stats["genuine"]["count"] == 3
    assert stats["genuine"]["min"] == 0.75
    assert stats["genuine"]["max"] == 0.9
    assert stats["impostor"]["count"] == 3
    assert stats["impostor"]["max"] == 0.3
    try:
        score_stats([], [])
    except ValueError:
        pass
    else:
        raise AssertionError("empty score sets must fail")


def test_probe_rows_match_official_summary_contract():
    from app.benchmark import build_probe

    probe = build_probe(
        expected="student-1", predicted=None, latency_ms=12.5, attack=False
    )
    assert probe == {
        "expected": "student-1",
        "predicted": None,
        "latency_ms": 12.5,
        "attack": False,
    }
    result = summarize([probe, build_probe(None, "leaked", 10.0, False)])
    assert result["known_probes"] == 1
    assert result["unknown_probes"] == 1
    assert result["fpir"] == 1.0
    assert result["fnir"] == 1.0
