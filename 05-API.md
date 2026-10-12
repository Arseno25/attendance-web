# 05 — HTTP & Realtime API Contract (Draft)

## 1. Conventions

Base path `/api/v1`; JSON except transient image upload `multipart/form-data`. All timestamps UTC RFC 3339; response includes `request_id`. User auth: server-side session cookie (`HttpOnly`, `Secure`, `SameSite=Lax/Strict`) with CSRF protection for state changes; RBAC checks on every endpoint. Kiosk access uses short-lived provisioned device session and operational policy (network/device restriction); **browser localStorage tokens are not hardware-secure credentials**. Python API is internal service-to-service only.

Pagination query: `page=1&page_size=20`, maximum 100; sorting allowlisted. Standard error:

```json
{
  "error": { "code": "VALIDATION_ERROR", "message": "NIS sudah digunakan", "fields": {"nis": "duplicate"} },
  "request_id": "a01152ee-034f-4aee-8e5a-9c008d8f9787"
}
```

## 2. Route matrix

| Method | Endpoint | Role | Purpose |
|---|---|---|---|
| POST | `/auth/login` | Public admin login | Create session |
| POST | `/auth/logout` | All logged-in | End session |
| GET | `/auth/me` | All logged-in | Session/role/scopes |
| GET | `/dashboard/summary?date=` | Admin, teacher scoped | KPIs |
| GET | `/students` | Admin, teacher scoped | Search / pagination |
| POST | `/students` | Admin | Add student |
| GET | `/students/{id}` | Admin, teacher scoped | Detail |
| PATCH | `/students/{id}` | Admin | Update |
| POST | `/students/imports/dry-run` | Admin | Validate file |
| POST | `/students/imports/{id}/commit` | Admin | Commit validated import |
| GET/POST | `/classes` | Scoped | Classes |
| GET/POST | `/academic-years` | Scoped | Academic years |
| GET | `/students/{id}/consents` | Authorized admin | Consent history |
| POST | `/students/{id}/consents` | Consent officer | Register validated legal status |
| POST | `/students/{id}/consents/withdraw` | Authorized workflow | Disable matching + queue deletion |
| POST | `/students/{id}/enrollments` | Enroll operator | Start enrollment (transient upload) |
| GET | `/students/{id}/face-status` | Admin/authorized operator | Status only |
| POST | `/kiosk/session` | Provisioned kiosk | Short-lived limited session |
| POST | `/kiosk/recognitions` | Kiosk session | Recognition of cropped temporary image |
| GET | `/attendance-sessions` | Admin/teacher scoped | Schedule sessions |
| GET | `/attendances` | Admin/teacher scoped | Records |
| POST | `/attendances/manual` | Authorized operator | Manually register attendance |
| POST | `/attendance-corrections` | Authorized user | Request correction |
| POST | `/attendance-corrections/{id}/decision` | Approver | Approve/reject |
| GET | `/devices` | Admin/operator | List devices |
| POST | `/devices` | Admin | Provision |
| POST | `/devices/{id}/revoke` | Admin | Revoke instantly |
| GET | `/reports/attendance` | Admin/teacher scoped | Report download/filter |
| GET | `/audit-logs` | Super admin only | Redacted audit trail |
| GET | `/ws/attendance` | Authorized admin/teacher | Realtime events |

## 3. Core payloads

### POST `/students`

```json
{
  "nis": "10245",
  "full_name": "Contoh Siswa",
  "status": "active",
  "class_id": "15b804f7-c8dd-418b-93c9-8169d729d088",
  "academic_year_id": "244d1a7a-cf34-4eec-ae71-9ae118a0f846"
}
```

Returns `201 Created` with generated UUID and enrollment, never face template.

### POST `/kiosk/recognitions` (multipart)

Fields: `image` temporary JPEG/WebP (size/dimension limit), `track_id`, `captured_at`, `event_id`, `session_id`. Device identity is taken from server-authenticated kiosk context, not a free-text `device_id` field. Replay protection uses bounded event UUID/nonce and TTL. Server strips EXIF and drops image bytes after inference; no CDN, S3 or body logging.

```json
{
  "request_id": "af8e7ad0-6789-439c-b574-6a4f6dff21b1",
  "decision": "accepted",
  "attendance": {
    "status": "present",
    "recorded_at": "2026-10-11T23:48:10Z",
    "already_recorded": false
  },
  "ui_message": "Absensi berhasil"
}
```

Unknown response (no candidate):

```json
{
  "decision": "unknown",
  "attendance": null,
  "ui_message": "Belum dapat dikenali. Silakan hubungi petugas."
}
```

No `student_id`, name, face embedding, or similarity score returned to **public kiosk**. Admin dashboard obtains authorized student details through its own session.

### POST `/attendances/manual`

```json
{
  "session_id": "d6739898-7919-4374-8bf5-7b574a461e13",
  "student_id": "32bd8a47-3d8f-47de-bc4f-5b0a39b7f857",
  "status": "present",
  "reason": "Verifikasi petugas saat kamera tidak tersedia"
}
```

The backend records actor and time; never trusts supplied `approved_by` or `created_at`.

## 4. AI internal contract

Internal `POST /internal/v1/recognize` accepts sanitized binary image, school scope, model version, and request ID from Go only. Example result:

```json
{
  "decision": "accepted",
  "candidate_student_id": "32bd8a47-3d8f-47de-bc4f-5b0a39b7f857",
  "model_version": "face-embed-v1",
  "liveness_result": "live",
  "quality_result": "pass",
  "stable_match": true,
  "latency_ms": 326
}
```

Python service is isolated from public internet; Go verifies all business conditions independently. Sensitive quality scores limited to internal testing with redaction.

## 5. WebSocket event

Channel: `/api/v1/ws/attendance`. Authentication via same authorized user session (validate origin) or short-lived WS auth token. Example:

```json
{
  "type": "attendance.created",
  "event_id": "8b11cfa9-2d5e-4cdf-ad15-e7f4b2d9a8ac",
  "school_id": "28d2c384-cd62-4822-9e79-1c2186a70fd0",
  "class_id": "15b804f7-c8dd-418b-93c9-8169d729d088",
  "attendance_id": "df5d21b2-4e86-44fe-b216-7ae50424ea8a",
  "status": "present",
  "occurred_at": "2026-10-11T23:48:10Z"
}
```

Publish only authorized school/class events. No raw biometric data via WebSocket. Reconnect means `GET /attendances?updated_since=...`, because events are not guaranteed durable.

## 6. Error mapping

- `400 VALIDATION_ERROR`: invalid field or frame.
- `401 UNAUTHENTICATED`: no valid session.
- `403 FORBIDDEN`: school/class/role mismatch.
- `404 NOT_FOUND`: resource not visible within scope.
- `409 CONFLICT`: duplicate NIS, concurrent state conflict.
- `410 CONSENT_WITHDRAWN`: not allowed to capture biometrics.
- `413 PAYLOAD_TOO_LARGE`: image limit.
- `422 FACE_QUALITY_LOW` / `SPOOF_SUSPECTED`: no auto-attendance.
- `429 RATE_LIMITED`: request budget exceeded.
- `503 AI_UNAVAILABLE`: fallback to manual.

## 7. Server protection

Allowlist origins and content types; rate-limit login, import and recognition; signed short-lived kiosk sessions; secure headers; bounded JSON/media; audit security changes; no sensitive data in URL query; protect exports with permission + logging; field-level response filtering. Use JSON schema/OpenAPI generation from handlers after implementation; this document is a specification draft, not a generated OpenAPI schema.
