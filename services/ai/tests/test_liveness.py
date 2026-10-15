from pathlib import Path

import numpy as np
import pytest


def test_pad_preprocessing_shifts_crop_at_edges_and_keeps_bgr():
    from app.liveness import prepare_crop

    frame = np.full((240, 320, 3), [10, 20, 30], np.uint8)
    for box in ([0, 0, 100, 100], [230, 150, 90, 90], [100, 80, 100, 100]):
        tensor = prepare_crop(frame, np.array(box, np.float32))
        assert tensor.shape == (1, 3, 80, 80) and tensor.dtype == np.float32
        assert tensor[0, :, 0, 0] == pytest.approx([10, 20, 30])
    with pytest.raises(ValueError):
        prepare_crop(frame, np.array([0, 0, 0, 100]))


def test_pad_uses_upstream_live_class_one_and_rejects_invalid_logits():
    from app.liveness import live_probability

    assert live_probability(np.array([[0, 10, 0]], np.float32)) > 0.99
    assert live_probability(np.array([[10, 0, 0]], np.float32)) < 0.01
    for values in (
        np.array([[np.nan, 1, 2]]),
        np.zeros((1, 2)),
        np.full((1, 3), np.inf),
    ):
        with pytest.raises(ValueError):
            live_probability(values)


def test_pinned_pad_model_cpu_smoke_is_not_accuracy():
    from app.liveness import LivenessModel

    directory = Path(__file__).resolve().parents[3] / ".tools" / "models"
    if not (directory / "minifasnet_v2.onnx").exists():
        pytest.skip("pinned PAD asset unavailable")
    pad = LivenessModel(directory)
    frame = np.zeros((240, 240, 3), np.uint8)
    result = pad(frame, np.array([40, 40, 150, 150]))
    assert type(result) is bool
