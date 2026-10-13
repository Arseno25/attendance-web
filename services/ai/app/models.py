"""Pinned CPU model installation; installation never authorizes recognition."""

import argparse
import hashlib
import json
import os
from pathlib import Path
from urllib.request import urlopen

import onnxruntime as ort

MANIFEST = Path(__file__).resolve().parents[1] / "models" / "manifest.json"


def verify_model(directory, spec):
    path = Path(directory) / spec["filename"]
    if (
        path.stat().st_size != spec["size"]
        or hashlib.sha256(path.read_bytes()).hexdigest() != spec["sha256"]
    ):
        raise ValueError("Model checksum mismatch")
    return path


def install(directory):
    directory = Path(directory)
    directory.mkdir(parents=True, exist_ok=True)
    manifest = json.loads(MANIFEST.read_text())
    result = []
    for spec in manifest["models"]:
        path = directory / spec["filename"]
        if not path.exists():
            temporary = path.with_suffix(".download")
            try:
                with (
                    urlopen(spec["url"], timeout=60) as source,
                    temporary.open("wb") as target,
                ):
                    total = 0
                    while chunk := source.read(1024 * 1024):
                        total += len(chunk)
                        if total > spec["size"]:
                            raise ValueError("Model exceeds pinned size")
                        target.write(chunk)
                downloaded = dict(spec, filename=temporary.name)
                verify_model(directory, downloaded)
                os.replace(temporary, path)
            finally:
                temporary.unlink(missing_ok=True)
        verify_model(directory, spec)
        license_path = directory / (spec["name"] + ".LICENSE")
        if not license_path.exists():
            with urlopen(spec["license_url"], timeout=30) as source:
                license_path.write_bytes(source.read(32 * 1024))
        options = ort.SessionOptions()
        options.intra_op_num_threads = 2
        options.inter_op_num_threads = 1
        session = ort.InferenceSession(
            str(path), sess_options=options, providers=["CPUExecutionProvider"]
        )
        result.append(
            {
                "name": spec["name"],
                "sha256": spec["sha256"],
                "inputs": [
                    {"name": v.name, "shape": v.shape} for v in session.get_inputs()
                ],
                "outputs": [
                    {"name": v.name, "shape": v.shape} for v in session.get_outputs()
                ],
            }
        )
    return {
        "source_commit": manifest["source_commit"],
        "models": result,
        "recognition_enabled": False,
        "pending": ["camera hold-out evaluation", "liveness evaluation", "calibration"],
    }


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--directory", required=True)
    args = parser.parse_args()
    print(json.dumps(install(args.directory), indent=2))
