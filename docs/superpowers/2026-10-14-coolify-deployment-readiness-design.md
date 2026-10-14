# Coolify Deployment Readiness Design

## Goal

Close three deployment gaps in the Coolify Compose stack: persist verified ONNX models independently of source checkout, gate API startup on schema migrations, and clearly specify a safe one-time first-admin bootstrap.

## Agreed decisions

- ONNX model storage uses a named Docker volume mounted at `/models`. Do not depend on a repository-relative host bind mount.
- Models continue to use the committed manifest's pinned URL, expected byte size, and SHA-256 validation. A one-shot init service populates the named volume; AI mounts it read-only and starts after successful init.
- Database migrations run automatically as an ordinary one-shot Compose service on each deployment reconciliation. The API depends on successful migration completion. Migration uses separate, required `MIGRATION_DATABASE_URL` credentials rather than the runtime `DATABASE_URL` role.
- Bootstrap is not part of recurring deploy startup. After migrations and API health, an operator runs the existing bootstrap CLI exactly once, passing the initial password over stdin. The existing CLI refuses a second bootstrap and does not log password material.

## Compose behavior

- Add an `init-models` service based on the existing AI build, using a writable `/models` mount and the pinned `app.models` installer. It must complete successfully before `ai` starts.
- Add a `migrate` service based on the existing API image, overriding the entrypoint/command to execute the bundled Goose binary against `/app/migrations`, using required migration credentials. It must complete successfully before `api` starts.
- Declare a named volume for model files and remove `AI_MODELS_HOST_PATH` bind-mount configuration from production Compose/example env.
- Preserve current AI runtime security: the AI service sees model files read-only, has no public port, and retains the existing internal token/key configuration.
- Use Compose `depends_on` with `condition: service_completed_successfully` for migration/API and init-models/AI. Validate that local Compose implementation accepts the syntax.

## Operator documentation

- Update deployment instructions and environment example to describe a required least-privilege migration URL, persistent model volume creation/backup/recovery, startup ordering, and model init behavior.
- Document initial admin bootstrap as a separate one-time post-migration operation using the existing image CLI and private stdin; do not provide a real/default production password or automate credentials.
- State limitations: local Compose validation does not prove Coolify deployment behavior; persistent volume still requires independent backups, and Coolify server test remains an operator release gate.

## Scope and files

- `deploy/compose.application.yml`: named model volume, model init service, migration service, startup dependencies, migration env.
- `deploy/.env.example`: migration connection template and remove source-path model setting.
- `scripts/test_compose_ports.py` or a focused deployment Compose test: assert named volume, read-only AI model mount, writable init mount, service commands/dependencies and migration role variable.
- `README.md`: correct deployment steps and initial administrator operation.
- `docs/LOCAL-RUNBOOK.md`: operational model volume and migration/bootstrap/restore notes as relevant.

No provider account, Coolify server, database, or production resource is modified by this work.

## Acceptance criteria

1. Application Compose config parses with local example configuration.
2. AI model path does not depend on a relative host checkout path; init has write access and AI runtime is read-only over the same named volume.
3. API cannot start unless Goose migration service exits successfully; migration service uses its separate required DB connection and the bundled migrations directory.
4. Bootstrap remains a manually invoked, one-time operation after migration; no password is embedded in Compose, image configuration, docs, or deploy command.
5. Tests assert the rendered Compose contract, and relevant Python tests and `git diff --check` pass.
6. Documentation accurately distinguishes locally verified Compose behavior from unverified live Coolify deployment.
