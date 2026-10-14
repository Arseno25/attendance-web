# Coolify Deployment Readiness Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Coolify Compose deploy with persistent verified AI models, successful migrations required before API startup, and a documented one-time administrator bootstrap.

**Architecture:** Add a named model volume populated by the existing pinned model installer in a one-shot init container; the AI runtime mounts the same volume read-only. Add a one-shot Goose service that gets a dedicated migration connection from Coolify secrets and gates API startup through Compose dependency health/completion ordering. Keep administrator creation manual and one-time, using the existing CLI with password supplied on stdin.

**Tech Stack:** Docker Compose, Coolify, Python model installer, bundled Goose CLI, Python unittest.

**Spec:** `docs/superpowers/specs/2026-10-14-coolify-deployment-readiness-design.md`

## Global Constraints

- Do not change or deploy Coolify/server/database resources.
- Model files remain pinned by manifest URL, byte size, and SHA-256 verification.
- AI mounts model storage read-only and exposes no host port.
- Migration credentials are separate from API runtime credentials and supplied as required `MIGRATION_DATABASE_URL`.
- Bootstrap is never part of recurring startup; never add a default production password or put passwords in command arguments/logs.
- Keep existing web host port mapping and local allowlist behavior unchanged.

## Review Focus

- Empty named model volume: the init service must have write permission and AI must wait for its successful exit.
- Existing named model volume: rerunning init must remain safe and verify existing files through the installer.
- Missing migration secret: Compose config must fail rather than use runtime DB credentials.
- Goose failure: API startup must be gated on successful migration completion.
- Coolify behavior: Compose config validation is local evidence only; live volume persistence, secret wiring, migration role privileges, and one-shot service reconciliation still require release verification on the target server.

---

### Task 1: Add persistent model initialization and migration startup gate

**Files:**
- Modify: `deploy/compose.application.yml`
- Modify: `deploy/.env.example`
- Test: `scripts/test_compose_ports.py`

**Interfaces:**
- Compose services: `init-models`, `migrate`, `ai`, `api`.
- Shared model volume: top-level Compose volume `model_data`, mounted at `/models` in both model services; the rendered Docker name is scoped by the Compose project name.
- Migration connection secret: required Compose variable `MIGRATION_DATABASE_URL`, passed only to Goose as `GOOSE_DBSTRING`.
- Goose command: bundled `goose -dir /app/migrations up`; existing API image already includes Goose and `/app/migrations`.

- [ ] **Step 1: Extend the rendered Compose test first**

Add tests asserting that the rendered application config has:
- `init-models` built from `../services/ai`, command `python -m app.models --directory /models`, a writable `model_data:/models` mount, and no restart loop.
- `ai` mounted from the same volume with `:ro`, with `depends_on.init-models.condition == service_completed_successfully`.
- `migrate` built from `../services/api`, environment `GOOSE_DRIVER=postgres` and the migration URL mapped to `GOOSE_DBSTRING`, no `DATABASE_URL` fallback, command `goose -dir /app/migrations up`, and no restart loop.
- `api.depends_on.migrate.condition == service_completed_successfully`.
- No relative `AI_MODELS_HOST_PATH` bind mount or old `AI_MODELS_HOST_PATH` env setting.

Keep the existing host-port rendering assertions unchanged.

- [ ] **Step 2: Run the test and verify it fails for missing deployment behavior**

Run: `python3 -m unittest scripts.test_compose_ports -v`
Expected: FAIL because the new services/volume/dependencies are absent, not due to test errors.

- [ ] **Step 3: Implement the Compose services and volume**

In `deploy/compose.application.yml`, define top-level `model_data: {}` without a fixed Docker `name`; Compose scopes it by project, so renaming the project creates a new volume and leaves the old one in place. Add `init-models` using the AI build context, `python -m app.models --directory /models`, and writable model volume. Run this ephemeral initialization service as UID 0 only to initialize an empty Docker-managed volume; downloaded files are created with normal readable file permissions. Set `restart: "no"`.

Add the same `model_data` volume to `ai` with `:ro`; add `depends_on` requiring successful `init-models` completion. Preserve AI token/key, healthcheck, limits, and no published host ports.

Add `migrate` using the API build context, `GOOSE_DRIVER=postgres`, required `${MIGRATION_DATABASE_URL:?...}` as `GOOSE_DBSTRING`, and command `goose -dir /app/migrations up`; set `restart: "no"`. Make `api` depend on successful `migrate` completion. Do not pass migration secrets to web or AI.

In `deploy/.env.example`, add a clearly marked placeholder connection for a separately provisioned migration role and remove `AI_MODELS_HOST_PATH`.

- [ ] **Step 4: Run Compose contract tests and verify green**

Run: `python3 -m unittest scripts.test_compose_ports -v`
Expected: all tests PASS, including existing port mapping behavior.

Run: `docker compose --env-file deploy/.env.example -f deploy/compose.application.yml config --format json`
Expected: valid normalized JSON with both completion dependencies, one named volume shared between init/AI, read-only AI mount, and Goose migration command/environment.

### Task 2: Document safe release and bootstrap operation

**Files:**
- Modify: `README.md`
- Modify: `docs/LOCAL-RUNBOOK.md`
- Modify: `deploy/.env.example` (only if Task 1 leaves an operator-facing variable ambiguous)

**Interfaces:**
- Operator provisions separate least-privilege runtime and migration PostgreSQL roles; this repository does not create external Coolify database roles.
- Operator invokes the `bootstrap` binary already present in the API image after migration and API startup, with password only over private stdin.

- [ ] **Step 1: Replace stale deployment order in README**

Update Coolify steps to explain automatic Compose migration before API via `service_completed_successfully`, named model volume initialization/retention, and setting the required migration URL in Coolify. State the DB administrator must grant the migrator only required DDL privileges and runtime role only required application privileges.

Document bootstrap as a manual one-time action after migrations/API are ready, using the `bootstrap` binary in the running API container and a private stdin prompt/pipe; no password in args, Compose, env examples, or logs. Mention repeated bootstrap is rejected by the existing CLI.

- [ ] **Step 2: Document model-volume and rollback/backup implications in the runbook**

Explain that first startup downloads only pinned/checksummed models into the named volume; AI reads it read-only. The named volume survives normal container redeploys but is host-local and is not a backup. Note that an empty/new volume requires upstream model downloads and the server must have outbound access; restoring/moving hosts requires backing up or reinitializing it.

- [ ] **Step 3: Review documentation against Compose behavior**

Check all commands, variable names, service names, run ordering, and password handling against Task 1. Ensure text explicitly says Coolify server behavior has not been tested and operator must validate volume persistence and migration failure gating on the target resource.

Run: `grep -nE 'AI_MODELS_HOST_PATH|sekali sebagai release task|MIGRATION_DATABASE_URL|model_data|bootstrap' README.md docs/LOCAL-RUNBOOK.md deploy/.env.example`
Expected: no obsolete host bind-mount instructions; new values and operations are described consistently.

### Task 3: Final deployment-contract verification

**Files:**
- No new files.

- [ ] **Step 1: Run Python deployment test and project Python script tests**

Run: `python3 -m unittest scripts.test_compose_ports -v`
Expected: all deployment assertions pass.

Run: `python3 -m unittest discover -s scripts -p 'test_*.py'`
Expected: all script tests pass; report any unrelated failures explicitly.

- [ ] **Step 2: Validate both configured Compose files and whitespace**

Run: `docker compose --env-file deploy/.env.example -f deploy/compose.application.yml config --quiet && git diff --check`
Expected: exit 0 and no whitespace errors.

- [ ] **Step 3: Review final artifact against acceptance criteria**

Confirm normalized Compose JSON shows an API migration completion dependency, AI init completion dependency, one named persistent volume shared with read-only AI access, required distinct migration DSN, and no source-relative model bind mount. Confirm docs explain bootstrap remains manual and that Coolify server verification is still outstanding.

---

## Self-review

- **Spec coverage:** Named persistent model volume and pinned init installer are Task 1; migration role and API gate are Task 1; required env example, manual bootstrap, volume backup/restore, and unverified-Coolify boundary are Task 2; all compose/test acceptance criteria are Task 3.
- **Step scan:** Each task has an asserted rendered contract before the corresponding Compose edit; docs are checked against concrete service names and variables. No production account or secret operation is included.
- **Interface consistency:** Both dependency conditions reference exact service names `init-models` and `migrate`; model volume is `model_data`; migration secret maps only to `GOOSE_DBSTRING`.
- **Review focus coverage:** Empty/existing volume and failed migration cases are addressed by completion dependencies and written contract checks; missing secret/no fallback is rendered config assertion; Coolify limitations are explicit. Live Coolify failure injection cannot be tested locally and remains a release gate.
- **Proportion:** Three tasks cover the Compose contract, operator guidance, and final validation without introducing helper scripts or application-code changes.
