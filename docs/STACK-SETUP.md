# Setup stack — dokumentasi, CLI dan versi

Verifikasi pada 12 Oktober 2026. Dokumentasi dicari melalui `resolve-library-id`, lalu `query-docs` Context7 per konsep. Versi package aktual juga diverifikasi melalui npm, uv/PyPI, Go module proxy, GitHub releases serta image registry; versi dokumentasi tidak dianggap otomatis sama dengan artifact yang tersedia.

## Stack yang dipasang

| Komponen | Versi fondasi | Penguncian |
|---|---|---|
| Node | 26.7.0 | Dockerfile dan CI |
| Nuxt | 4.6.1 | package.json exact + package-lock.json |
| Vue / Vue Router | 3.5.43 / 5.4.0 | package.json exact + lockfile |
| Tailwind CSS | 4.3.3 | package.json exact + lockfile |
| shadcn-vue / shadcn-nuxt | 2.8.2 | Generated components + exact dependencies |
| Nuxt ESLint / ESLint | 1.17.0 / 10.12.0 | Dev dependencies exact |
| TypeScript / vue-tsc | 6.0.3 / 3.3.13 | Latest compatible compiler API verified by build/typecheck |
| Go | 1.27.2 | go.mod, Dockerfile, CI |
| pgx | 5.11.0 | go.mod + go.sum |
| Go x/crypto (Argon2id) / x/sys | 0.58.0 / 0.49.0 | go.mod + go.sum (WP-02) |
| Goose CLI | 3.29.0 | Makefile/Dockerfile pinned install |
| uv | 0.13.0 | Official installer, Dockerfile and CI |
| Python | 3.14.8 | .python-version, Dockerfile, CI; project limits to 3.14 |
| FastAPI / CLI | 0.143.0 / 0.0.32 | uv.lock |
| Uvicorn | 0.54.0 | uv.lock |
| ONNX Runtime CPU | 1.31.0 | uv.lock; no pretrained model downloaded |
| PostgreSQL | 18.6 | Separate local database image |
| govulncheck / pip-audit | 1.9.0 / 2.10.1 | CLI versions pinned in CI |

Runtime/package versions are selected from available stable artifacts. TypeScript 7.0.2 was tried and failed: it no longer exposes the JavaScript compiler API used by Vue compiler-sfc, vue-tsc and typescript-eslint. Pinning 6.0.3 resolved lint/typecheck/build without suppressing type checks. Python 3.14 has ONNX wheels; an unsupported newer Python ABI was not forced. PostgreSQL 19.0 image was not available at verification; PostgreSQL 18.6 was pulled and executed successfully.

## Official scaffolding commands used

```bash
npm create nuxt@latest apps/web -- --template minimal --packageManager npm --no-install --no-gitInit --no-modules
```

In `apps/web`:

```bash
npm install
npm install -D --save-exact tailwindcss@latest @tailwindcss/vite@latest typescript@latest vue-tsc@latest
npx nuxi@latest module add shadcn-nuxt
npx nuxi@latest module add eslint
npx shadcn-vue@latest init --defaults --base-color zinc --yes
npx shadcn-vue@latest add button card badge --yes
npm install -D --save-exact typescript@6.0.3
```

After install, actual versions were pinned and lockfiles retained. Future setup uses `npm ci`, not re-running `@latest`. Registry defaults produced the reka-nova neutral theme; CSS font changed to system fonts to avoid third-party font requests. Only primitives used by the starter page were added.

Go module setup in `services/api`:

```bash
GOTOOLCHAIN=go1.27.2 go mod init attendance.local/api
go get github.com/jackc/pgx/v5/pgxpool@latest
go get golang.org/x/text@latest golang.org/x/sync@latest
go mod tidy
```

`attendance.local/api` is an internal monorepo module path, not a claimed GitHub repository. No remote repository was fabricated. Goose was installed through official Go CLI with only the PostgreSQL driver enabled, then `goose -dir migrations -s create foundation sql` generated the initial migration.

WP-02 menambahkan Argon2id melalui `go get golang.org/x/crypto@latest` lalu `go mod tidy`. CLI `npx shadcn-vue@2.8.2 add input label alert table --yes` menghasilkan primitives untuk auth/admin. Context7 dipakai untuk pgx transactions, Nuxt middleware/useState/fetch dan shadcn forms. Dokumentasi Go Argon2 tidak tersedia dengan match yang tepat di Context7; implementasi mengikuti [package resmi Argon2](https://pkg.go.dev/golang.org/x/crypto/argon2). Dependency frontend tetap exact dan font tetap lokal setelah scaffold.

Python project setup through uv:

```bash
uv init services/ai --app --python 3.13 --vcs none
uv add fastapi fastapi-cli 'uvicorn[standard]' onnxruntime
uv add --dev pytest httpx2 ruff
uv python pin 3.14.8
uv sync --locked
```

The project was updated to `app/main.py`, package=false, and Python 3.14 after confirming available ONNX wheels. The obsolete generated hello entry point was removed. `fastapi[standard-no-fastapi-cloud]` was not an available extra on the installed artifact; FastAPI, its CLI and Uvicorn were installed explicitly to avoid unnecessary cloud tooling. Tests use httpx2 as recommended by the current installed Starlette TestClient.

Local `make setup` uses the official uv installer with `UV_INSTALL_DIR=.tools` and `UV_NO_MODIFY_PATH=1`; no shell profile is modified.

## Sources verified through Context7

| Concept | Context7 library | Official source |
|---|---|---|
| Nuxt create/prepare/typecheck/app structure | `/websites/nuxt_4_x` | [Nuxt init](https://nuxt.com/docs/4.x/api/commands/init), [TypeScript](https://nuxt.com/docs/4.x/guide/concepts/typescript) |
| shadcn + Tailwind v4 + Nuxt aliases | `/websites/shadcn-vue` | [Nuxt installation](https://shadcn-vue.com/docs/installation/nuxt) |
| Nuxt ESLint module/flat config | `/nuxt/eslint` | [Module docs](https://github.com/nuxt/eslint/blob/main/docs/content/1.packages/0.module.md) |
| Go modules | `/websites/go_dev_doc` | [Go dependency management](https://go.dev/doc/modules/managing-dependencies) |
| pgx/pool install | `/jackc/pgx` | [pgx getting started](https://github.com/jackc/pgx/wiki/Getting-started-with-pgx) |
| Goose CLI/migrations | `/pressly/goose` | [Goose README](https://github.com/pressly/goose) |
| uv installer/project/lock/Docker | `/astral-sh/uv` | [uv installation](https://docs.astral.sh/uv/getting-started/installation/), [Docker guide](https://docs.astral.sh/uv/guides/integration/docker/) |
| FastAPI CLI/testing | `/websites/fastapi_tiangolo` | [FastAPI CLI](https://fastapi.tiangolo.com/fastapi-cli/), [Testing](https://fastapi.tiangolo.com/advanced/testing-events/) |
| Uvicorn production flags | `/websites/uvicorn_dev` | [Settings](https://uvicorn.dev/settings/), [Logging](https://uvicorn.dev/concepts/logging/) |
| ONNX CPU package/providers | `/websites/onnxruntime_ai` | [Python installation](https://onnxruntime.ai/docs/get-started/with-python.html) |
| PostgreSQL Docker/volume layout | `/websites/hub_docker_postgres` | [Official image](https://hub.docker.com/_/postgres) |
| Compose health/build/start | `/docker/compose` | [Compose source and CLI](https://github.com/docker/compose) |
| Coolify cross-resource network | `/coollabsio/coolify-docs` | [Networking](https://github.com/coollabsio/coolify-docs/blob/main/content/docs/core/networking-in-coolify.mdx) |
| GitHub Actions workflows | `/websites/github_en_actions` | [Workflow tutorial](https://docs.github.com/en/actions/tutorials/create-an-example-workflow) |
| Dependency/secret scanning | `/golang/vuln`, `/pypa/pip-audit`, `/gitleaks/gitleaks` | [govulncheck](https://github.com/golang/vuln), [pip-audit](https://github.com/pypa/pip-audit), [Gitleaks](https://github.com/gitleaks/gitleaks) |

## Security findings and limits

Go scan initially found vulnerabilities in pgx's transitive `golang.org/x/text` 0.29.0. Upgrading x/text to 0.43.0 and x/sync to 0.24.0 passed tests and govulncheck. Python's locked runtime dependencies passed pip-audit at verification.

npm's critical simple-git chain was updated using an explicit `simple-git: 4.0.2` override. Lint/typecheck/build passed after the override. npm audit still reports upstream high/moderate findings, including braces, node-forge and decode-uri-component through Nuxt/registry tooling. Registry artifacts did not provide compatible patched releases for those findings; the suggested forced remediation would downgrade Nuxt or shadcn. These are tracked, not suppressed as clean. CI retains the full audit artifact; resolve or formally assess before production release.

No existing secrets, production credentials, model licenses, VPS hardware benchmark, TLS/network or Coolify deployment has been assumed verified. Container base tags are pinned to versions; image digest locking and image vulnerability assessment belong to release hardening WP-11.
