# Security and UI remediation implementation plan

> **For agentic workers:** execute inline with executing-plans and TDD; one fresh review after integration.

**Goal:** Repair S1–S4 and U1–U4 from the accepted security/UI audit, add web hardening and shorten recognition transactions without changing real attendance data.
**Architecture:** Keep existing service boundaries and shadcn composition. Centralize class bounds, restrict assignment administration to school scope, reuse live RBAC for kiosk credentials, and revoke delegated device sessions on authorization changes. Issue device-bound short-lived capture permits and reject repeated image hashes before inference; revalidate credentials/gallery after inference before saving.
**Tech Stack:** Go/pgx/PostgreSQL, FastAPI/OpenCV, Nuxt/Vue/shadcn-vue, installed test tooling only.
**Spec:** docs/SECURITY-UI-AUDIT-2026-10-15.md (user approved all findings for repair).

## Global constraints

- Preserve real school/student/face/attendance records. Test writes use disposable schemas.
- Keep model thresholds and camera calibration; do not claim physical spoof resistance without a camera evaluation.
- No framework replacement or new runtime dependencies. Reuse TableActions, PageHeader and semantic theme tokens.
- Local execution only; staging/Coolify credentials and deployment remain outside the approved environment.

## Review focus

- A scoped actor must not move an invisible student, import into another class, or self-extend class assignments.
- Revocation, natural assignment expiry, session closure and gallery changes during slow inference must prevent writes.
- Capture permits must be device-bound, expire, be single-use, and not allow the same image with fresh metadata.
- Security headers must allow Nuxt/color-mode inline startup, local cross-origin API, charts and the camera without permitting framing.
- Narrow screens, long names and nested action groups must keep all buttons visible; no data-table page slicing or false attendance success.

### Task 1: live authorization and revocation

Files: masterdata/students.go, assignments.go, auth/{policy,transaction,roles,users,profile}.go, attendance/devices.go; existing Go integration fixtures; web auth type/academics UI if necessary.
Interfaces: classBounds returns no rows outside scope; auth.UserCan(ctx,tx,userID,permission,classID) reuses loadAccess; kioskAuth enforces current operator permission/class.
- [x] Add failing integration cases for placements/import/assignments, pairing scope and permission revocation; run against local disposable schemas.
- [x] Apply guards and credential revocation in shared paths, preserve valid school and assigned-class operations.
- [x] Run auth/masterdata/attendance race integration suite and record results.

### Task 2: capture freshness and inference isolation

Files: new migration 00013, attendance/{capture,recognize,service}.go, AI recognition.py, absensi.vue/types, existing HTTP/component tests and OpenAPI.
Interfaces: POST /kiosk/capture-challenge issues event_id valid for 10 seconds for the current device session; recognize consumes it once and rejects duplicate encoded images retained 24 hours. AI additionally rejects identical decoded pixels in bounded device context. Recognition snapshots gallery in a short transaction and checks the live revision again after inference.
- [x] Add failing capture expiry/device/reuse tests, decoded replay test, and slow-inference state/concurrency tests.
- [x] Implement permit/retention, frontend permit-before-capture, canonical pixel duplicate check and two short transactions.
- [x] Update synthetic fixtures to supply fresh frames/permits, keeping explicit replay tests.
- [x] Run relevant Go/Python/Node suites; document residual trusted-capture/PAD limits.

### Task 3: web security and UI

Files: Nuxt server middleware/plugin and small header helper; PageHeader, devices/student action groups, TableActions/dataTable type, roles.vue, AttendanceTrendChart, index.vue; browser regression test.
Interfaces: response CSP uses a request nonce for Nuxt startup scripts; headers deny framing and restrict sources to self/configured API. Shared actions wrap within available width; role delete uses shared icon action; chart shows fewer ticks on narrow containers.
- [x] Add meaningful browser regressions for known overflows, row actions and CSP/bootstrap behavior, verify failures.
- [x] Implement minimal shared layout/security changes and stable public copy; preserve confirmation dialogs.
- [x] Build and verify real API browser fixtures at five widths, light/dark, dialogs and active kiosk synthetic presentation.

### Task 4: operational checks and completion evidence

Files: compose comments/runbook/OpenAPI/audit remediation notes; existing scanner policy.
- [x] Run make check/build, DB race tests, browser suite, dependency/secret checks; inspect every result.
- [x] Run controlled slow-AI concurrency tests and local image/runtime configuration checks without touching live student data.
- [x] Refresh local services after successful verification, inspect current UI, record screenshots and update the audit with fixed vs externally unverified items.
- [x] Perform fresh review of the resulting diff; repair material findings and rerun affected checks. Do not claim remote TLS/backup/physical PAD checks were completed locally.

Completion evidence: docs/SECURITY-UI-AUDIT-2026-10-15.md, hasil remediasi 16 Oktober 2026. Physical PAD, target-host controls and full load evaluation remain explicitly unverified; local code remediation is complete.
