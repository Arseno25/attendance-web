# Dynamic RBAC Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace static role permissions and per-user grants with school-scoped dynamic roles, audited superadmin management, and backend-enforced class/school scope.

**Architecture:** PostgreSQL stores school roles, role permissions and generic user/class assignments. The Go API owns the permission allowlist, live authorization, migration compatibility, audit and superadmin-only role endpoints; Nuxt consumes the catalog and role records for a focused admin UI. Existing `users.role` remains a stable role key, while all relevant endpoint checks and queries are converted from role-name branches to database-backed permissions and scope.

**Tech Stack:** Go 1.27, PostgreSQL, Goose SQL migrations, pgx/v5, Nuxt 4.6, Vue 3, shadcn-vue 2.8, TypeScript, Node `node:test`, existing Playwright/local browser harness where available.

**Spec:** `docs/superpowers/specs/2026-10-14-rbac-design.md`

## Global Constraints

- Use a new Goose migration; never edit an already-applied migration.
- Keep the permission identifier catalog application-owned and allowlisted; never accept arbitrary permission keys.
- Keep `super_admin` role key stable, scope `school`, and require `users.read`, `users.write`, and `roles.manage`; seed it with every catalog permission.
- Roles and class assignments are school-scoped; every SQL read/write continues to bind the configured school ID.
- Do not retain direct user permission grants or grant overrides; do not broaden a legacy user's effective access during migration.
- Preserve sessions, CSRF/Origin, audit append-only, class/year interval, biometric consent/revoke, and attendance-history invariants.
- Use existing Go, PostgreSQL, Nuxt, Vue, shadcn-vue and project test patterns; do not add dependencies.
- UI strings are Indonesian; route/API permission denial remains backend-enforced.

## Review Focus

- A legacy user with a unique grant combination or revoked/missing class assignment must migrate without receiving another user's permission or class access; pin in Task 1 migration tests.
- A role scope/permission update concurrent with an authorized write or stale session must not keep old privileges live; pin in Task 2 authorization/session integration tests.
- A custom role with `roles.manage`, `users.write`, or an unrelated permission must not gain superadmin-only management or out-of-scope resources; pin in Tasks 2 and 3.
- A report/dashboard filter naming another assigned-class ID must not leak records even if the caller has the corresponding report permission; pin in Task 4 tests.
- Editing/removing a role, API failure, or a narrow mobile viewport must not leave a false saved state, delete an in-use role, or produce an inaccessible/overflowing dialog; pin in Task 5 UI tests.

---

## File Map

### Database and Go API

- Create `services/api/migrations/00008_dynamic_rbac.sql` — role and permission tables/seeds, safe legacy-grant grouping, FK change, generic class-assignment rename, role/scope constraints and audit-preserving migration.
- Create `services/api/internal/auth/roles.go` — catalog metadata, role API handlers, role query/validation, audit and session invalidation for role changes.
- Modify `services/api/internal/auth/policy.go` — role/scope/permission model and deny-by-default checks; remove static permission maps and per-user grant behavior.
- Modify `services/api/internal/auth/service.go` — register role/catalog routes and validate role keys against the school database.
- Modify `services/api/internal/auth/users.go` — remove grants payload/storage paths, use DB roles, protect last active Superadmin, invalidate affected sessions.
- Modify `services/api/internal/auth/sessions.go`, `transaction.go`, and `profile.go` — resolve live role permissions/scope and effective class IDs from DB for session and transaction authorization.
- Modify `services/api/internal/masterdata/service.go`, `assignments.go`, `academics.go`, `students.go`, `consents.go`, `biometrics.go`, `imports.go`, `integration_test.go`, and `biometrics_integration_test.go` — enforce generalized assignment-based scope and replace teacher-name access branches.
- Modify `services/api/internal/attendance/service.go`, `sessions.go`, `records.go`, `corrections.go`, `checkout.go`, `schedules.go`, `reports.go`, `devices.go`, `live.go`, and `integration_test.go` — enforce permission and scope throughout attendance, reporting/export, devices and websocket handlers.
- Modify `services/api/internal/auth/security_test.go`, `integration_test.go`, `profile_integration_test.go`, `services/api/internal/masterdata/integration_test.go`, `services/api/internal/masterdata/biometrics_integration_test.go`, and `services/api/internal/attendance/integration_test.go` — update role fixtures and add policy/API/scope regression coverage.
- Create `services/api/internal/auth/rbac_migration_integration_test.go` in Task 1 — migration-specific fixture coverage using the existing disposable-schema setup pattern; Task 2 extends that existing file with role/auth endpoint integration coverage.

### Nuxt panel

- Create `apps/web/app/pages/admin/roles.vue` — superadmin role listing/editor with grouped permission catalog, scope, confirmation and states.
- Modify `apps/web/app/types/auth.ts` — dynamic role key and response types; remove per-user `grants` and static role label map as an authority.
- Modify `apps/web/app/components/AppSidebar.vue` and `apps/web/app/components/SiteHeader.vue` — role-management nav/breadcrumb.
- Modify `apps/web/app/pages/admin/users.vue` and `apps/web/app/lib/validation/auth.ts` — load role list and select a role; remove per-user grant controls and validation.
- Modify `apps/web/app/pages/admin/academics.vue` — generic user class assignment UI and copy while preserving existing tabs, modal, and revocation behavior.
- Modify `apps/web/app/pages/admin/account.vue` and `apps/web/app/components/NavUser.vue` — display role name from dynamic role metadata or a stable session role label without a stale fixed enum.
- Create `scripts/test_rbac_ui.mjs` — source-component regressions for menu gating, role editor behavior, dynamic user role selection and no grant payload.

## Task 1: Migrate role and assignment data without access expansion

**Files:**
- Create: `services/api/migrations/00008_dynamic_rbac.sql`
- Create: `services/api/internal/auth/rbac_migration_integration_test.go` — ordered migration upgrade and legacy access/scope preservation fixtures; Task 2 will extend this file for auth/role API integration coverage.
- Test: PostgreSQL disposable-schema integration test run via `TEST_DATABASE_URL`.

**Interfaces:**
- Produces schema objects consumed by later tasks: `roles(school_id,id,key,name,description,scope,is_system,created_at,updated_at)`, `role_permissions(school_id,role_id,permission)`, and `user_class_assignments` preserving old assignment identity/history.
- Keeps `users.role` as key with same-school FK; removes `permission_grants` only after all old permission sets have been represented.
- Permission keys seeded: `users.read`, `users.write`, `roles.manage`, `audit.read`, `school.write`, `students.read`, `students.write`, `academics.read`, `academics.write`, `attendance.read`, `attendance.manual`, `attendance.approve`, `corrections.request`, `reports.export`, `devices.manage`, `devices.health`, `biometrics.enroll`, `biometrics.revoke`, `consents.manage`.

- [ ] **Step 1: Add migration fixture test for built-in roles and legacy permission grouping**

  Build a disposable migrated database with Superadmin, baseline admin/teacher/operator, two accounts with the same extra-permission set, one account with a different set, active/inactive accounts, active/revoked teacher assignments, then assert role assignments preserve each pre-migration effective permission set and scope exactly. Assert identical `(legacy role, scope, effective permission set)` tuples share a migration role; distinct tuples never share one; no account gets cross-school or unassigned-class access. Assert Superadmin starts with the complete permission catalog and mandatory permissions.

- [ ] **Step 2: Run the migration fixture against current schema and confirm it fails for missing dynamic schema/behavior**

  Run: `cd services/api && TEST_DATABASE_URL="$RBAC_DISPOSABLE_DATABASE_URL" go test ./internal/auth -run TestRBACMigrationPreservesLegacyAccess -count=1`

  Expected: FAIL because dynamic role tables and migration behavior are absent.

- [ ] **Step 3: Implement the additive migration and deterministic legacy conversion**

  In `00008_dynamic_rbac.sql`, add tenant-keyed role/permission constraints and indexes, seed the 19 allowlisted permissions and built-in roles for existing schools, derive legacy permissions from the current four static role sets plus valid grants, group non-Superadmin users by exact set/scope into collision-safe deterministic migration roles, move assignment rows preserving IDs and revocation history, replace the role CHECK with same-school FK, and only then remove `permission_grants`. Ensure initialization for future schools is exposed for Task 2 bootstrap rather than relying on a migration that has already run.

- [ ] **Step 4: Run migration test and formatting/schema checks**

  Run: `cd services/api && TEST_DATABASE_URL="$RBAC_DISPOSABLE_DATABASE_URL" go test ./internal/auth -run TestRBACMigrationPreservesLegacyAccess -count=1`

  Expected: PASS; all old effective permissions/scopes are preserved, no broader role is assigned, legacy grant table is absent, and assignment identities/history match.

- [ ] **Step 5: Verify test runner compatibility on all API migrations**

  Run: `cd services/api && TEST_DATABASE_URL="$RBAC_DISPOSABLE_DATABASE_URL" go test ./internal/auth -run 'TestAuthDatabaseLifecycleAndPermissions|TestRBACMigrationPreservesLegacyAccess' -count=1`

  Expected: PASS on a confirmed disposable PostgreSQL database; never substitute preview/production. If the variable is unset, integration tests report a skip and this is not counted as verification.

## Task 2: Database-backed auth, role API, and user assignment

**Files:**
- Create: `services/api/internal/auth/roles.go`
- Modify: `services/api/internal/auth/policy.go`, `service.go`, `users.go`, `sessions.go`, `transaction.go`, `profile.go`, `security_test.go`, `integration_test.go`, `profile_integration_test.go`, and `rbac_migration_integration_test.go` (the Task 1 migration test file).

**Interfaces:**
- `type RoleScope string` with `ScopeSchool` and `ScopeAssignedClasses`.
- `type Permission struct { Key, Label, Description, Group string }`; `PermissionCatalog() []Permission` returns the stable app allowlist.
- `type Role struct { ID, Key, Name, Description string; Scope RoleScope; IsSystem bool; Permissions []string; UserCount int }` with JSON names per the spec.
- `User` carries `Role` key, resolved `RoleName`, effective permission keys and `Scope` internally; it no longer carries `Grants`. JSON session/user responses include `role_name` for every role so non-superadmin account/profile UI can show custom role names without accessing superadmin-only `/roles`. `User.Can(permission, schoolID, classID)` denies unknown permissions, school mismatch, inactive accounts, and out-of-scope class access.
- `seedBuiltinRoles(ctx context.Context, tx pgx.Tx, schoolID string) error` idempotently seeds the four built-in roles and current app catalog for bootstrap/new schools; migration uses the same built-in permission/scope definitions.
- `Service.Register` exposes GET `/api/v1/permissions`, GET/POST `/api/v1/roles`, PATCH/DELETE `/api/v1/roles/{id}`.
- `POST/PATCH /users` keeps `role` key and drops `grants`; login/session response keeps `permissions` as the effective key array.

- [ ] **Step 1: Write Go policy tests for dynamic role permissions and scope invariants**

  Assert custom-role allow/deny, unknown-permission denial, school isolation, assigned-class allow only for the assigned class, no grant-based bypass, mandatory Superadmin permissions/scope, and custom `roles.manage` not equaling system Superadmin identity.

- [ ] **Step 2: Run focused policy tests and observe expected failures**

  Run: `cd services/api && go test ./internal/auth -run 'TestPolicyUsesDynamicRole|TestRoleManagementIsSuperadminOnly|TestAssignedClassScope' -count=1`

  Expected: FAIL because current policy is a fixed map and has no dynamic scope model.

- [ ] **Step 3: Implement catalog and pure policy model in `policy.go`**

  Add the defined `RoleScope`/`Permission` types and complete catalog with Indonesian display metadata; make effective permissions explicit on the loaded user. Enforce `super_admin` system identity for security-only checks and keep class scope independent of feature permission.

- [ ] **Step 4: Implement role CRUD handlers and DB authorization snapshot**

  In `roles.go`, implement stable list ordering, school-only SQL, catalog response, strict JSON decoding, name/scope/allowlist validation, immutable keys, mandatory Superadmin core permissions/scope, in-use/system role deletion conflict, transactional audit, role-wide session invalidation, and safe PG error mapping. Permission catalog/create/update/delete all require actual `super_admin` role, not merely `roles.manage`.

- [ ] **Step 5: Remove direct grants and make user/session authorization role-backed**

  Change `users.go` request/response, SQL columns and session invalidation to role references only. Load permissions/scope from the role tables in `sessions.go` and after transaction locks in `transaction.go`; compute assigned classes from the generic assignment table only when the loaded role scope is `assigned_classes`. Keep bootstrap idempotency/first-admin semantics while seeding built-ins before creating Superadmin. Validate role keys against current school and deny unavailable roles. Keep the Superadmin self-edit permission allowed subject to mandatory keys, and keep protection against disabling/reassigning the last active Superadmin.

- [ ] **Step 6: Add integration tests for CRUD, superadmin gates, audit, and revocation**

  Test each endpoint with a real Superadmin session and a non-superadmin session, including a custom role that has `roles.manage`; invalid/duplicate/unknown permissions and invalid scope; role deletion in use; cross-school IDs; auditing in same transaction; role update revokes sessions for every assigned user; future-school bootstrap seeds roles; user JSON has no `grants`; role selection accepts only existing same-school key.

- [ ] **Step 7: Run focused auth tests**

  Run: `cd services/api && go test -race ./internal/auth -count=1`

  Expected: PASS, including migration, auth session/profile, permission policy, role CRUD, and concurrency checks against a configured disposable database.

## Task 3: Generalize class assignments and enforce scope in master data

**Files:**
- Modify: `services/api/internal/masterdata/service.go`, `assignments.go`, `academics.go`, `students.go`, `consents.go`, `biometrics.go`, `imports.go`, `integration_test.go`, and `biometrics_integration_test.go`.

**Interfaces:**
- Assignment APIs: GET/POST `/api/v1/class-assignments`, PATCH `/api/v1/class-assignments/{id}`, GET `/api/v1/class-assignment-users`; request identity field `user_id`; response includes generic `user_id`/`user_name` with existing class/year/interval/status fields.
- `auth.User.ClassIDs` is live effective class scope; masterdata `scope` consumes it without using a role name.
- Existing `academics.read`/`academics.write`, `students.read`/`students.write`, `consents.manage`, and biometric permissions continue to define actions. Any SQL scope indicator is `RoleScope`/`ClassScoped`, never the role key.

- [ ] **Step 1: Add failing custom-role masterdata scope integration coverage**

  Create two custom users with the same class-scoped permission set, assign one to class A only, and assert list/detail/write/import/consent/biometric actions stay within A; assert another role with school scope sees permitted records in A and B; assert class assignment only accepts active users whose current role scope is `assigned_classes`. In `services/api/internal/masterdata/biometrics_integration_test.go`, assert an enrol/revoke operator with assigned-class scope cannot discover or mutate students outside its assignments.

- [ ] **Step 2: Run focused masterdata integration tests and observe failures**

  Run: `cd services/api && TEST_DATABASE_URL="$RBAC_DISPOSABLE_DATABASE_URL" go test ./internal/masterdata -run 'TestDynamicRoleClassScope|TestGenericClassAssignments' -count=1`

  Expected: FAIL because handlers and queries still hardcode teacher role and `/teacher-assignments`.

- [ ] **Step 3: Replace masterdata assignment API/queries and teacher-only options**

  Update `assignments.go` for generic user targeting and stable historical results, dynamic role scope filtering, permission-gated options, audit labels, and append-preserving revoke. Update `academics.go` and `service.go` to route the generic endpoints. Convert SQL filters from `role='teacher'` to resolved `scope` and actor class IDs. Keep the existing UI-consumed academic-year boundaries and assignment validation unchanged.

- [ ] **Step 4: Apply `User.ClassIDs` to every masterdata resource**

  Review each students, academic year/class, placement, import, consent/guardian, biometric list/status/enroll/revoke query and write. Class-linked data must constrain source/target class in SQL/transaction. Avoid imposing class scope on features without class-linked records; always apply school ID. Preserve biometric consent eligibility and purge rules. Extend `biometrics_integration_test.go` to prove a class-scoped enrolment/revoke operator cannot discover or mutate a student outside assigned classes.

- [ ] **Step 5: Update integration tests and run masterdata suite**

  Migrate fixtures to dynamic roles and generic assignments, add cross-class biometric operator denial coverage in `biometrics_integration_test.go`, retain regression cases for prior assignments and history, then run:

  Run: `cd services/api && go test -race ./internal/masterdata -count=1`

  Expected: PASS; custom class roles match selected assignment scope, revoked assignments cease authorizing, historical identities remain, and no cross-school references pass.

## Task 4: Enforce permission and scope across attendance, reports, devices, and realtime

**Files:**
- Modify: `services/api/internal/attendance/service.go`, `sessions.go`, `records.go`, `corrections.go`, `checkout.go`, `schedules.go`, `reports.go`, `devices.go`, `live.go`, and `integration_test.go`.

**Interfaces:**
- Attendance service continues calling `auth.BeginAuthorized`; returned `auth.User` has effective permissions, role scope, and current class IDs.
- Report and dashboard SQL bind class IDs for every class-scoped role (not only `teacher`); report permissions control JSON/CSV independently of scope.
- `attendance.manual` special roster/correction behavior is explicit and cannot bypass assigned-class constraints.

- [ ] **Step 1: Add failing attendance/report tests for custom roles and scope**

  Assert custom assigned-class user sees its assigned sessions/participants/reports/CSV only; another class returns empty/404 per existing resource visibility contract; role scope school sees both permitted classes; revoked/expired assignments immediately remove visibility; devices remain controlled by device permission rather than class assignment; non-permitted websocket sessions are denied; kiosk device credentials remain isolated from user-role permission changes except the existing pairing/revocation flow.

- [ ] **Step 2: Run targeted attendance tests and observe current hardcoded-role failures**

  Run: `cd services/api && TEST_DATABASE_URL="$RBAC_DISPOSABLE_DATABASE_URL" go test ./internal/attendance -run 'TestDynamicRoleAttendanceScope|TestDynamicRoleReportScope|TestDynamicRoleDevicePermission' -count=1`

  Expected: FAIL while report SQL and role gates depend on teacher/admin/operator names.

- [ ] **Step 3: Convert attendance resource guards and scope SQL**

  Update schedule/session/roster/record/checkout/correction handlers to rely on catalog permissions and authorized class IDs. Preserve approval separation, operator roster limits, session snapshot behavior, write locks, consent-independent manual fallback, and attendance history. Remove role-name branches that are authorization decisions; only identity-specific Superadmin restrictions remain where specifically required.

- [ ] **Step 4: Convert report/dashboard/export and websocket scope**

  Change `reportFilter.args`, `reportSelect`, dashboard totals/recent and CSV authorization to use role scope rather than `u.Role == teacher`. Verify explicit requested class IDs cannot expand scope. Keep CSV audit, formula escaping, limits, websocket invalidation semantics, and live session checks.

- [ ] **Step 5: Run attendance and full Go suites**

  Run: `cd services/api && go test -race ./internal/attendance -count=1 && go test -race ./...`

  Expected: PASS, with all existing attendance invariants and new permission/scope behavior passing. If database integration requires `TEST_DATABASE_URL`, provision only a project-local disposable test database using repository runbook; never migrate the preview/production database for tests.

## Task 5: Add clean superadmin role management UI and dynamic user/assignment forms

**Files:**
- Create: `apps/web/app/pages/admin/roles.vue`
- Modify: `apps/web/app/types/auth.ts`, `apps/web/app/components/AppSidebar.vue`, `apps/web/app/components/SiteHeader.vue`, `apps/web/app/pages/admin/users.vue`, `apps/web/app/lib/validation/auth.ts`, `apps/web/app/pages/admin/academics.vue`, `apps/web/app/pages/admin/account.vue`, `apps/web/app/components/NavUser.vue`.
- Create: `scripts/test_rbac_ui.mjs`.

**Interfaces:**
- TypeScript `RoleScope = 'school' | 'assigned_classes'`; `Permission`, `Role` and API envelope types match Task 2 JSON names exactly. Session/user responses expose dynamic `role_name`; account/NavUser display that server-provided label, falling back to built-in Indonesian labels and then a neutral unknown-role label. Superadmin user editor resolves role keys through its separately fetched role list.
- Role page uses `/permissions` and `/roles`; user editor uses `/roles`; class-assignment tab uses `/class-assignment-users` and `/class-assignments`.
- Navigation item text `Role & permission`, route `/admin/roles`, category `Administrasi`; middleware is `['auth','super-admin']`.

- [ ] **Step 1: Add failing UI source tests for sidebar gate, editor rules, and user contract**

  Compile production Vue SFCs using the repository's Node source-test pattern. Assert only Superadmin sees the new route, grouped allowlisted permission metadata renders, Superadmin mandatory core keys cannot be unchecked, role edit payload includes selected keys/scope only, failed save preserves dialog/form without success state, delete conflict retains role, user-create/update payload has no grants, and class assignment selects `user_id` options from dynamic scope.

- [ ] **Step 2: Run the new UI test and confirm expected failures**

  Run: `node --test scripts/test_rbac_ui.mjs`

  Expected: FAIL because the route/page and dynamic role form are not implemented.

- [ ] **Step 3: Add types and role route, data table, and editor**

  Define exact API interfaces in `types/auth.ts`; implement `/admin/roles` using existing `PageHeader`, `DataTable`, `Card`, `Dialog`, `AlertDialog`, `Field`, `Select`, `Checkbox`, `Badge`, `Spinner`, and Sonner patterns. Load catalog/roles with loading/error/empty states; keep selected values stable across edit; display permission groups, scope, role-system marker and affected-user/session warning. Enforce mandatory Superadmin permissions client-side for usability while relying on API for truth.

- [ ] **Step 4: Wire navigation, breadcrumb, and dynamic user role choice**

  Add Superadmin-only sidebar route and breadcrumb. Replace role enum/static map in user editor with API role options, remove grants fields/validation/checkboxes from user payload and screen, show dynamic role label, and refresh/error-handle role lookup. Keep self-edit restriction and user table paging/search behavior.

- [ ] **Step 5: Wire generic academic assignment form and role display**

  Change teacher-only assignment tab to `Penugasan kelas`; select active class-scoped users and submit `user_id` to generic endpoints while preserving time bounds, modal validation, status/action handling, and revoke flow. Update account/profile display to use the current role label metadata safely (unknown role renders a neutral fallback, never a crash). Avoid static permission decisions in role labels/menu logic.

- [ ] **Step 6: Run UI source test, lint, formatting, and typecheck**

  Run: `node --test scripts/test_rbac_ui.mjs && npm --prefix apps/web run lint && npm --prefix apps/web run format:check && npm --prefix apps/web run typecheck`

  Expected: PASS with dynamic role flow and no stale role/grant references in production components.

## Task 6: End-to-end regression, browser verification, and plan closure

**Files:**
- Modify as required from preceding tasks only: affected Go/SQL/frontend/test files.
- Verify: spec criteria and all paths in this plan.

**Interfaces:**
- Full service starts through existing `make dev-api`/`make dev-web`; test DB follows `Makefile` `TEST_DATABASE_URL` convention.
- No product contract beyond Tasks 1–5.

- [ ] **Step 1: Search for stale static authorization and grant references**

  Search `services/api/internal` for `rolePermissions`, `GrantAllowed`, `permission_grants`, `role='teacher'`, `role='admin'`, `u.Role ==`, and `u.Role !=`; each remaining result must be migration-only, display-only, or a documented security identity guard. Search `apps/web/app` for `grants`, `roleLabels`, and hard-coded role arrays; remove permission/route behavior based on stale static enums.

- [ ] **Step 2: Run complete repository checks**

  Run: `make check && make build`

  Expected: PASS (Go race tests/vet/format, Python, Node, Nuxt lint/format/typecheck/build, API/bootstrap builds). Preserve failures and fix root causes; do not suppress or skip assertions.

- [ ] **Step 3: Verify migration and API in disposable PostgreSQL**

  Set both `DATABASE_URL` and `TEST_DATABASE_URL` explicitly to the same confirmed disposable PostgreSQL database, then run `make db-test`. Verify the DSN/database identity before running because the Make target applies `services/api/tests/foundation.sql` directly to `DATABASE_URL`; never point it at the preview or production database. If no disposable DB is available, do not run this target and report integration migration verification as blocked; unit/build checks are not a substitute. Verify migration up, role routes, non-superadmin 403s, school isolation, user role update, grant absence and session revocation.

- [ ] **Step 4: Verify browser workflow through the actual panel**

  Run the local dev services and browser harness. As Superadmin, create a class-scoped custom role, assign permissions, create a user with that role, assign the user to one class, and verify permitted/unpermitted student and attendance/report screens. Edit the role permission and scope; verify audit and login/session renewal behavior. As a non-superadmin, attempt direct route and API access and verify denial. Check create/edit/cancel/error, mandatory Superadmin permission interaction, deletion conflict, keyboard/dialog focus and 390px viewport without horizontal page overflow. Do not perform destructive actions against real identities or non-disposable project data.

- [ ] **Step 5: Reconcile acceptance criteria and report**

  Compare each spec criterion and each Review Focus item to a passing test/browser observation. Report exact checks that passed, skipped or could not run, plus migration/deployment limitations. Do not claim production migration, UAT, or release readiness from local tests.

## Self-review checklist (plan author)

- **Spec coverage:** Tasks 1–2 cover dynamic schema, built-ins, superadmin mandatory keys, catalog/API, bootstrap, users without grants, auditing and session revocation. Tasks 3–4 cover generic assignments, tenant/class scope, all masterdata/attendance/report/device/realtime call sites and preserved domain controls. Task 5 covers sidebar, superadmin page, user editor, assignment UI, responsive/accessibility behavior. Task 6 covers stale-reference review, complete checks, disposable migration/API and browser verification.
- **Step scan:** Each task starts with a failing behavior test, implements only its owning boundary, and ends with a focused passing check; final build/browser checks do not replace integration assertions.
- **Type consistency:** Shared Go types and JSON fields are defined in Task 2 and used by later handlers/tests; `RoleScope` values and API paths/payloads are identical across tasks; frontend types mirror the documented response keys.
- **Review Focus:** All five listed hazards have explicit tests in Tasks 1–5.
- **Proportion:** The plan groups changes by testable boundary; it does not prescribe function bodies where the approved spec leaves implementation freedom. The wide endpoint list is necessary because static role checks are distributed across all those files.
