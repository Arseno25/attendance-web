# Face reference photo and shadcn badge implementation plan

> Execute inline with `executing-plans`; one independent review after verification.

**Goal:** Show one enrollment reference photo in face management and after verified attendance, and repair shared badge rendering against the official shadcn-vue registry.

**Spec:** User request of 16 October 2026: one registration image for visual verification, consistent shadcn components and badge styles.

**Architecture:** AI crops and encrypts one JPEG from the first validated registration frame. Nullable columns on the existing face template share its lifecycle; legacy templates remain usable without a photo. The admin photo read rechecks school/class permissions after decryption. Kiosk receives a photo only in an approved recognition response, never through a public student-image endpoint.

**Stack:** Existing Nuxt/Vue/shadcn-vue, Go/pgx, FastAPI/OpenCV/AES-GCM. No new runtime dependency.

## Constraints and review focus

- Preserve the existing class scope, PAD/stability/threshold checks, and attendance idempotency.
- Never log/store raw registration frames; persist only one encrypted 192×192 JPEG.
- Revocation, student deactivation, and replacement remove the old photo with the template.
- No name/photo exposure for unknown, unconfirmed, or wrong-class recognition.
- No network decryption while holding database authorization locks.
- Legacy rows show a neutral fallback; no fabricated/reconstructed portrait.
- Do not replace the existing student's enrollment merely to populate the feature.

## Tasks

- [x] 1. Compare/install shared Badge and Avatar via official CLI. Keep official base/default variants; diagnose actual rendering and retain centralized semantic status variants only where needed. Inspect desktop/mobile in both themes.
- [x] 2. Add failing crypto/enrollment tests, then implement encrypted photo creation and authenticated decryption in `services/ai/app/reference_photo.py`, `pipeline.py`, `main.py`; test tampering, identity/key binding, dimensions and no image echoes.
- [x] 3. Add migration 15 and Go client/metadata/admin photo route. Test atomic enrollment, class denial, revocation, legacy fallback and authorization recheck after delayed AI response. Attach a reference only after runtime-approved recognition and release SQL locks before decryption.
- [x] 4. Reuse shadcn Avatar in a reusable `StudentFacePhoto.vue`; add table/modal preview and kiosk result portrait, update copy/types/OpenAPI. Add browser coverage with synthetic fixtures for approved/unknown and legacy cases.
- [x] 5. Run full checks, database integration/race tests, build and browser tests; apply the additive migration and refresh local services; inspect actual UI once plus one confirmation round if needed. Independent final review, fix material findings, report legacy-photo limitation.

## Execution notes

- Existing enrollment contains embeddings only; no old registration image can be recovered.
- User authorized retaining one reference image. Other 3–5 source frames remain ephemeral.
- Existing uncommitted work stays intact; no seed/reset, merge/push, deployment or threshold changes.

## Verification results

- Context7 + shadcn-vue MCP + latest CLI registry/diff inspected; existing components retained. Official destructive Badge restored; semantic status styles remain shared, face label/score truncation repaired.
- `make check`: passed; 87 AI tests, 66 Node tests, 5 Python script tests, Go unit/race checks, lint/format/typecheck.
- Real Postgres integration with `-race`: masterdata, attendance and auth passed, including post-decryption class/template/device revocation.
- Browser suite: 19/19 passed. Distinct synthetic A/B portraits, unknown responses, legacy no-photo fallback, table/modal and mobile bounds covered. The legacy transition exposed and fixed Reka Avatar's retained loaded state.
- Production web build and static API build passed; additive migration 15 and all local services refreshed, readiness 200. Existing real enrollment and attendance retained.
- Actual UI inspected in light/dark and mobile/desktop. Theme restored to System and viewport reset. Existing student has no historical portrait; update enrollment to populate one. No real camera enrollment replaced, no new accuracy/PAD claim.
- Fresh read-only review found a kiosk post-decryption authorization race; regression failed before the fix and passed after. Device and template are now reauthorized after the external call, outside SQL locks. No unresolved material findings within this change.
