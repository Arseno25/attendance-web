# Attendance AI

Service internal FastAPI dengan ONNX Runtime CPU. Entry point `app.main:app`. `uv.lock` mengunci dependencies; `.python-version` memilih Python 3.14.8.

```bash
../../.tools/uv sync --locked
../../.tools/uv run fastapi dev --host 127.0.0.1 --port 8001
../../.tools/uv run pytest -q
```

Recognition sengaja disabled. Model/license/liveness/threshold/gallery akan diintegrasikan pada WP-06–WP-09. Header `Authorization: Bearer ...` harus berisi secret `AI_SERVICE_TOKEN` setidaknya 32 karakter. Tidak ada model, template, gambar atau storage wajah pada fondasi ini.

Container production menggunakan CLI Uvicorn dengan access logs disabled, concurrency terbatas dan proxy headers tidak dipercaya. Jangan beri service ini domain publik.
