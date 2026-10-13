# Attendance AI

Internal FastAPI service on Python 3.14 with pinned YuNet and SFace CPU models. `app.main:app` is the entry point; `uv.lock` pins OpenCV headless, ONNX Runtime and cryptography. The model manifest pins source commit, license, size and SHA-256; model files are mounted read-only.

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

`POST /internal/v1/recognize` returns actual normalized detection boxes. It does not identify people or create attendance: there is no verified liveness implementation or independent authorized camera hold-out data. A detected single face returns `unknown`; multiple faces return `inconclusive`. Candidate IDs and names are absent. `GET /internal/v1/status` and readiness report pipeline availability separately from `recognition_enabled: false` and `MODEL_LIVENESS_NOT_EVALUATED`. Installation and synthetic model smoke tests are not accuracy evaluation.

The Go gallery is built from live active-student, unexpired/unwithdrawn consent, tombstone and attendance roster joins for every request. AI retains no template/gallery cache. Withdrawal, revocation and inactive-student transitions remove ciphertext transactionally; a minute worker removes expired ciphertext. A template key must be retained to read its templates: replacing the single configured key requires re-enrollment; there is no key rotation tool in this pilot.

Evaluation remains **belum dievaluasi** until a responsible officer supplies authorized independent camera hold-out data and a suitable licensed liveness implementation. The offline exact cosine matcher in `app.pipeline.match_embedding` accepts explicit calibrated threshold/margin, a liveness result and at least three stable observations. Synthetic unit checks prove rejection logic, not operational accuracy. Runtime has no environment switch to bypass the missing G2 evidence.

After obtaining authorized evaluation inputs:

1. Keep capture/enrollment frames separate from held-out known-person probes. Include unknown people, lighting/occlusion conditions, camera changes and photo/video presentation attacks. Use anonymous subject IDs; do not put names, NIS, student UUIDs or image data in decision traces.
2. Evaluate the chosen liveness implementation and track stability with the real camera protocol. Tune threshold and runner-up margin using a calibration split, then freeze them before the independent test split. The provisional enrollment consistency/blur filters need calibration too.
3. Produce JSON decision traces as an array of `{ "expected": "anonymous-subject" | null, "predicted": "anonymous-subject" | null, "latency_ms": number, "attack": boolean }`. These must be observed test decisions, not fabricated success records.
4. Run `../../.tools/uv run --locked python -m app.evaluation /private/path/to/authorized-hold-out-trace.json`. The report separates known FNIR/wrong identity, unknown FPIR, attack pass rate, p95 latency and the 95% upper bound when unknown false accepts are zero. The report deliberately does not approve automatic attendance. Review model/license/calibration/liveness/latency evidence and implement the validated runtime pipeline before enabling it.

Current API acceptance wiring additionally requires every evaluated gate, a report SHA and matching evaluated-model SHA, then rechecks live consent, roster, active student and open session before using server time and recording an idempotent check-in. Browser track UUID is a correlation hint, never liveness or identity evidence. The present AI endpoint never emits this accepted contract.
