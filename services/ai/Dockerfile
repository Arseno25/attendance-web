FROM ghcr.io/astral-sh/uv:0.13.1 AS uv
FROM docker.io/library/python:3.14.8-slim
COPY --from=uv /uv /usr/local/bin/uv
ENV UV_COMPILE_BYTECODE=1 UV_LINK_MODE=copy UV_NO_CACHE=1 PYTHONUNBUFFERED=1
WORKDIR /app
COPY pyproject.toml uv.lock .python-version ./
RUN uv sync --locked --no-dev
COPY app ./app
COPY models/manifest.json ./models/manifest.json
RUN useradd --system --uid 10001 --create-home app
USER app
ENV PATH="/app/.venv/bin:$PATH"
EXPOSE 8001
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8001", "--no-proxy-headers", "--no-access-log", "--no-server-header", "--limit-concurrency", "16"]
