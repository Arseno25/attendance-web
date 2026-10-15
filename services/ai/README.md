# Attendance AI

Internal FastAPI service on Python 3.14 with pinned YuNet, SFace and MiniFASNet CPU models. `app.main:app` is the entry point; `uv.lock` pins OpenCV headless, ONNX Runtime and cryptography. The model manifest pins source commit, license, size and SHA-256; model files are mounted read-only.

```bash
../../.tools/uv sync --locked
../../.tools/uv run --locked pytest -q
../../.tools/uv run --locked ruff check app tests
```

Runtime configuration:

| Variable | Purpose |
| --- | --- |
| `AI_MODELS_DIR` | Read-only model directory, `/models` in containers |
| `AI_SERVICE_TOKEN` | Service-to-service bearer secret, at least 32 characters |
| `AI_TEMPLATE_KEY_BASE64` | Base64 encoding of a 32-byte AES-256 key, held outside the database |
| `AI_TEMPLATE_KEY_ID` | Non-secret key identifier, e.g. `local-v1` |

Go uses the same service token and an operator-configured `AI_BASE_URL`, e.g. `http://ai:8001`. Never accept this URL from a browser request. No redirect is followed; the internal network request has a six-second timeout. Do not expose the AI service on a public domain. Container access logging is disabled and concurrency is bounded.

`POST /internal/v1/enroll` accepts a server-established school/student/consent scope and 3–5 distinct JPEG frames. It bounds encoded bytes and JPEG dimensions before decode, requires one sufficiently large and sharp face in each frame, aligns the face and extracts normalized SFace embeddings. A provisional consistency filter rejects mismatched samples. It returns only AES-GCM ciphertext, random nonce and metadata; authenticated additional data binds school, student, consent, recognizer SHA and key ID. Frames and plaintext vectors stay in process memory and are never saved or logged.

`POST /internal/v1/recognize` uses the encrypted school/session/device-scoped gallery and the tracking engine. A single live face must match consistently over three observations within five frames, pass the configured similarity/margin and PAD checks, and remain eligible in the Go roster. Missing/failed PAD, ambiguity, poor quality, multiple faces and unknown people never create attendance. Go supplies the roster name and commits attendance once; recognition cannot change an existing administrative status. Offline evaluation metadata is separate from operational readiness and is never fabricated.

The Go gallery is built from active students, current templates and the attendance session roster for every request. Guardian consent is retired. Legacy consent IDs remain encryption context only. AI keeps at most eight school/session/device contexts and resets confirmation when gallery content changes. Revocation and inactive-student transitions remove ciphertext transactionally. A template key must be retained to read its templates: replacing the single configured key requires re-enrollment; there is no key rotation tool in this pilot.

Evaluation remains **belum dievaluasi** until a responsible officer supplies authorized independent camera hold-out data and a suitable licensed liveness implementation. The offline exact cosine matcher in `app.pipeline.match_embedding` accepts explicit calibrated threshold/margin, a liveness result and at least three stable observations. Synthetic unit checks prove rejection logic, not operational accuracy. Offline accuracy evidence is distinct from runtime checks; no fabricated report or always-true liveness flag is used to enable recognition.

After obtaining authorized evaluation inputs:

1. Keep capture/enrollment frames separate from held-out known-person probes. Include unknown people, lighting/occlusion conditions, camera changes and photo/video presentation attacks. Use anonymous subject IDs; do not put names, NIS, student UUIDs or image data in decision traces.
2. Evaluate the chosen liveness implementation and track stability with the real camera protocol. Tune threshold and runner-up margin using a calibration split, then freeze them before the independent test split. The provisional enrollment consistency/blur filters need calibration too.
3. Produce JSON decision traces as an array of `{ "expected": "anonymous-subject" | null, "predicted": "anonymous-subject" | null, "latency_ms": number, "attack": boolean }`. These must be observed test decisions, not fabricated success records.
4. Run `../../.tools/uv run --locked python -m app.evaluation /private/path/to/authorized-hold-out-trace.json`. The report separates known FNIR/wrong identity, unknown FPIR, attack pass rate, p95 latency and the 95% upper bound when unknown false accepts are zero. The report deliberately does not approve automatic attendance. Review model/license/calibration/liveness/latency evidence and implement the validated runtime pipeline before enabling it.

Current API acceptance wiring additionally requires every evaluated gate, a report SHA and matching evaluated-model SHA, then rechecks live consent, roster, active student and open session before using server time and recording an idempotent check-in. Browser track UUID is a correlation hint, never liveness or identity evidence. The present AI endpoint never emits this accepted contract.

## Reusable recognition engine

`app.face_engine.FaceRecognitionEngine` is an internal recognition-only engine separate from the public attendance endpoint. It reuses the SHA-verified CPU YuNet/SFace pipeline. `FaceDetector`, `FaceRecognizer`, `FaceMatcher` and `RecognitionSession` handle detection, normalized 128-dimensional features, vectorized exact cosine matching and temporal confirmation. Gallery scores aggregate the best of 1–5 templates per identity; the runner-up is a different identity. Threshold and margin must be supplied explicitly after calibration.

The default session requires three consistent observations within five frames, samples at most every 200 ms, resets on disappearance, multiple faces, quality/identity changes, errors and gallery mutations, invalid/stale frames, and never repeats a cached identity during cooldown. Session count, frame age and timeouts are bounded. The optional trusted PAD hook reports evidence; rejection or exceptions clear confirmation. Missing PAD remains `None`, never a liveness pass. A caller must isolate galleries by school and maintain live student/session eligibility when loading encrypted templates. `load_embeddings`, `update_student_embeddings`, `remove_student` and `reset_session` are explicit operations; this module does not own a database, HTTP authentication or attendance.

`AI_OPENCV_THREADS` bounds OpenCV CPU threads (integer 1–16, default 2). The shared detector serializes threshold/input-size changes across callers. Enrollment/preview retain their existing detection threshold and quality rules; preview never extracts embeddings. `POST /internal/v1/face-preview` is service-token protected and returns only boxes, count and quality. The public Go bridge additionally checks current student, assigned classes, role, Origin and CSRF.

```bash
# From services/ai. Synthetic performance only, not accuracy or camera FPS.
../../.tools/uv run --locked python -m app.engine_tools benchmark --models ../../.tools/models --identities 1000 --iterations 30
# Requires an independently captured, authorized local dataset.
../../.tools/uv run --locked python -m app.engine_tools calibrate --models ../../.tools/models --dataset /private/calibration.json --authorized-dataset --threshold 0.8 --margin 0.1
../../.tools/uv run --locked python -m app.engine_tools evaluate --models ../../.tools/models --dataset /private/holdout.json --authorized-dataset --threshold 0.8 --margin 0.1
```

Threshold/margin values above demonstrate syntax only, not operating recommendations. Calibration runs an exploratory sweep; freeze chosen settings before evaluating a separate hold-out split. The JSON manifest uses paths relative to its directory:

```json
{
  "split": "holdout",
  "model_sha256": "<actual SFace SHA from model manifest>",
  "gallery": {"anon-one": ["gallery/one-a.jpg", "gallery/one-b.jpg"]},
  "probes": [
    {"expected": "anon-one", "frames": ["holdout/one-1.jpg", "holdout/one-2.jpg", "holdout/one-3.jpg"]},
    {"expected": null, "frames": ["unknown/1.jpg", "unknown/2.jpg", "unknown/3.jpg"]}
  ]
}
```

Use `split: calibration` for calibration. Each track requires 3–32 distinct frames; duplicate gallery images and gallery/probe overlap are rejected. Anonymous IDs are required. Reports derive actual engine decisions and stage timings, unknown rejection, FNIR/FPIR and latency; no result approves automatic attendance. Use --with-pad and include print/replay attack tracks to evaluate PAD; recognition-only reports cannot establish spoof resistance. Sampling delay is disabled for supplied offline tracks; these timings cannot establish live-camera frame rate, timestamp tracking or resistance to replay. Enrollment and quality filters also still need representative real-camera calibration.

## Fantech camera and runtime

Enrollment and `/absensi` share the existing `FaceCamera` component. The shadcn-vue camera selector prefers Fantech and remembers an explicitly selected device in this browser. A fresh permission grant may be needed before labels appear. Stop the camera before switching devices. Only video is requested.

MiniFASNet input follows the actual [upstream tensor conversion](https://github.com/minivision-ai/Silent-Face-Anti-Spoofing/blob/master/src/data_io/functional.py): BGR float32 values remain 0–255. This ONNX export starts with Conv and contains no input normalization. Live is class 1 per upstream test.py. The exporter card's normalization/label guidance conflicts with upstream; follow the executable source, retain camera calibration and do not claim measured accuracy without a separate dataset.
