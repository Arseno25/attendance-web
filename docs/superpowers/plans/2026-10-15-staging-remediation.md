# Staging remediation implementation plan

> **For agentic workers:** Use executing-plans inline. Audit findings are approved for implementation by the user. Keep the existing architecture and use ponytail ultra.

**Goal:** Fix the staging audit findings, update supported stable dependencies, and make the remaining face-pilot gates explicit and testable.

**Architecture:** Reuse Go service structs/methods, Vue composables and Python inference classes. Authorization belongs on the server; browser permissions mirror it. Preserve existing public data and require evidence before enabling automatic face attendance.

**Tech Stack:** Nuxt/Vue/shadcn-vue, Go/net/http/pgx, PostgreSQL, FastAPI/OpenCV/ONNX Runtime; exact stable versions checked against official registries. Context7 sources: Nuxt upgrade, FastAPI model lifecycle, Playwright browser testing.

**Spec:** `docs/STAGING-READINESS-AUDIT-2026-10-15.md`, findings 1–10 and face feature gaps; user authorizes repairs and current stable stack.

## Constraints

- One school and one API replica; no new service layers or broad pattern rewrites.
- Do not reset application data, restore retired consent requirements, expose secrets, deploy, or push.
- Use class scoping consistently; trusted proxy headers require configured proxy CIDRs.
- Automatic face decisions remain fail-closed without validated PAD and independent camera evaluation.
- Preserve the pre-existing local SCHOOL_ID removal; commit the final tested revision on a local feature branch.

## Review focus

- No class assignments and foreign-class IDs must not grant biometric access.
- Spoofed forwarded headers must not change identity for untrusted peers.
- Calendar changes must affect unopened sessions without rewriting attendance history.
- An approver may decide another person's request in their assigned class, never their own.
- Per-role browser paths and real cross-origin DELETE must work through a real browser.

## Tasks

### 1. Server class access and lifecycle rules

- [x] Add failing database integration regressions for biometric list/status/preview/enroll/revoke class scope, assigned-class approval, and holiday-before-open API/worker.
- [x] Fix shared helpers in `internal/masterdata/biometrics.go`, `internal/attendance/corrections.go`, `sessions.go`, and `worker.go`; reuse student scope conditions.
- [x] Run the relevant Go suites against disposable schemas; verify public data is unchanged.

### 2. HTTP and login proxy boundary

- [x] Add failing tests for DELETE preflight, untrusted/spoofed headers, trusted proxy chains, IPv4/IPv6 and separate login clients.
- [x] Add a small net/http client-address resolver using net/netip, explicit trusted proxy CIDRs from config, and thread it to authentication; include DELETE in CORS.
- [x] Update environment/Compose/runbook, run Go HTTP/auth/config tests.

### 3. Permission-based web UI

- [x] Add failing middleware/action tests for custom roles with only required student/report permissions.
- [x] Align menu, route guards, report actions and student forms with effective server permissions; retain reusable existing composables.
- [x] Run Node UI tests, lint and typecheck.

### 4. Face integration and duplicate registration

- [x] Add failing tests for encrypted gallery scoping, duplicate enrollment, inference route integration and unevaluated fail-closed results.
- [x] Wire the existing engine through a bounded service instance, include scoped encrypted templates for duplicate checking, and provide a reproducible hold-out runner/calibration path.
- [x] Research a maintained licensed PAD provider; install only verified assets and keep approval disabled until real evaluation passes.
- [x] Run AI and Go integration suites; do not invent accuracy or treat fake test data as a camera evaluation.

### 5. Stable dependencies, fresh install and browser regression

- [x] Check official stable package versions, upgrade compatible packages with official CLIs, and triage advisories against the final runtime image/output.
- [x] Correct bootstrap order and prove a fresh isolated database can migrate → bootstrap → start API; test repeat bootstrap/migration failures.
- [x] Add minimal real-browser cross-origin login/photo/permission/report smoke coverage, plus CI wiring with disposable data.
- [x] Document trusted proxy/TLS, staged load test, backups/restore and exact unresolved external gates.

### 6. Verify and package

- [x] Run the full lint/type/test/build, Compose and dependency checks on the final tree.
- [x] Run fresh-context final review and address substantive findings with regressions.
- [x] Update audit status and runbook with evidence; commit the exact local release revision and retain all pre-pull recovery snapshots.
- [x] Report remaining camera/Coolify/load gates without claiming staging deployment.
