# UPGRADE V2.0 — IMPLEMENTATION REPORT

Status: **DONE** (scope kode) / **NEEDS HUMAN APPROVAL** (kelayakan produksi biometrik)

Laporan ini disusun setelah audit nyata terhadap source code, implementasi perbaikan
(P0–P2 yang feasible pada lingkungan ini), dan eksekusi test matrix aktual. Dokumen
perencanaan terkait: `docs/UPGRADE-V2-PLAN.md`.

---

## 1. Executive Summary

Upgrade V2.0 menempatkan sistem absensi wajah sekolah pada kondisi yang **stabil,
teruji, dan fail-closed** untuk operasi biometrik:

- **AI Operation Mode Gate** (`AI_OPERATION_MODE`): `disabled` | `shadow` | `approved` |
  `provisional`. Mode `approved` hanya aktif jika laporan evaluasi independen cocok
  hash-nya, konfigurasi engine cocok, liveness lulus, dan disetujui manusia; selain itu
  otomatisasi absensi **fail-closed**.
- **Filter status pendaftaran wajah** (`enrolled=true|false`) pada `GET /api/v1/students`
  dan `GET /api/v1/biometrics/students`, plus flag `face_enrolled` pada daftar siswa.
- **Ringkasan operasional dashboard** (`operations` pada `GET /api/v1/dashboard/summary`):
  sesi berjalan, kamera online/total/dicabut, kesiapan pipeline, dan mode operasi AI.
- **State eksplisit kiosk** (`deriveKioskState` + `kioskStateLabels`): 12 state termasuk
  `multiple_faces`, `inconclusive`, `network_error`, `session_inactive` — tanpa animasi berat,
  tanpa menampilkan identitas sebelum backend mengonfirmasi.
- **Kontrak OpenAPI** (`docs/openapi/master-data.json`, `biometrics.json`, `reports.json`)
  disinkronkan dengan implementasi aktual.
- **Konfigurasi deployment** (`deploy/compose.application.yml`, `deploy/.env.example`)
  memperlihatkan `AI_OPERATION_MODE` secara eksplisit dengan default `provisional`.

Seluruh perubahan di atas didorong oleh tes yang ditulis lebih dahulu (TDD: RED → GREEN),
dan seluruh test suite yang ada tetap lulus tanpa ada assertion yang dilemahkan.

---

## 2. Audit Findings (ringkas)

Temuan lengkap ada di `docs/UPGRADE-V2-PLAN.md`. Ringkasan gap utama yang diperbaiki
pada iterasi ini:

| Gap | Severity | Status |
|---|---|---|
| GAP-V2-01 AI Operation Mode Gate belum ada | P0 | DONE |
| GAP-V2-02 Filter `enrolled` / flag `face_enrolled` belum ada | P1 | DONE |
| GAP-V2-03 Ringkasan operasional (sesi/kamera/AI) pada dashboard | P1 | DONE |
| GAP-V2-04 Kontrak OpenAPI belum sinkron | P1 | DONE |
| GAP-V2-05 Deployment belum memperlihatkan operation mode | P1 | DONE |
| GAP-V2-06 State kiosk tidak eksplisit | P1 | DONE |

Gap lain (biometric governance/consent lifecycle yang lebih dalam, benchmark 500 identitas,
E2E Playwright pada hardware nyata, evaluasi liveness dengan dataset eksternal) tetap
terbuka dan tercatat pada bagian **Remaining Risks / Blockers**.

---

## 3. Perubahan yang Berhasil Diimplementasikan

### 3.1 AI Operation Mode Gate (P0)

- `services/ai/app/pipeline.py`:
  - `operation_mode()` membaca `AI_OPERATION_MODE` (`disabled`, `shadow`, `approved`,
    `provisional` — default `provisional`).
  - Gate keputusan recognition sebelum commit attendance:
    - `disabled`: tidak ada recognition/attendance otomatis.
    - `shadow`: evaluasi berjalan, tanpa mencatat attendance otomatis.
    - `approved`: hanya izinkan attendance otomatis jika approval artefak lolos
      (`approval.load_approval` cocok hash + config + liveness + review manusia).
    - `provisional`: mode pengembangan dengan data uji berizin; tetap fail-closed untuk
      klaim produksi.
- `services/ai/app/main.py`: memperlihatkan `operation_mode` pada `/internal/v1/health`
  dan hasil recognition.
- `services/ai/tests/test_recognition_service.py`: `test_operation_modes_control_automatic_recognition_and_fail_closed`
  menguji keempat mode + fail-closed.

### 3.2 Face Enrollment Filter & Flag (P1)

- `services/api/internal/masterdata/students.go`:
  - `studentSelect` menyertakan `EXISTS(...) AS face_enrolled`.
  - `GET /api/v1/students` mendukung `?enrolled=true|false` (validasi: `""|true|false`,
    selain itu `400`).
- `services/api/internal/masterdata/biometrics.go`:
  - `GET /api/v1/biometrics/students` mendukung `?enrolled=true|false`, tetap mempertahankan
    6 field minimal (privasi selector biometrik tidak diperluas).
- `services/api/internal/platform/aiclient/client.go`: `Status` menyertakan `operation_mode`.
- Tes regresi: `services/api/internal/masterdata/biometrics_integration_test.go`
  (`enrolled=true` mengembalikan siswa terdaftar, `enrolled=false` mengecualikannya,
  `enrolled=invalid` → `400`).

### 3.3 Dashboard Operational Summary (P1)

- `services/api/internal/attendance/reports.go` (`dashboard`):
  - Menambahkan `operations`: `open_sessions`, `total_devices`, `online_devices`,
    `revoked_devices`, `pipeline_ready`, `recognition_enabled`, `operation_mode`.
- Tes regresi: `services/api/internal/attendance/integration_test.go`.
- Frontend: `apps/web/app/types/reports.ts` (`DashboardOperations`),
  `apps/web/app/pages/admin/index.vue` (kartu sesi berjalan, kamera, layanan wajah,
  tabel Aktivitas Absensi Terbaru dengan empty state shadcn-vue).

### 3.4 Kiosk State Machine (P1)

- `apps/web/app/types/biometrics.ts`: `KioskState` (12 state).
- `apps/web/app/lib/biometrics.ts`: `deriveKioskState(...)` (pure function) +
  `kioskStateLabels`.
- `apps/web/app/pages/absensi.vue`: badge status kiosk real-time (`Menyiapkan perangkat`,
  `Kamera siap`, `Mendeteksi wajah`, `Memproses identifikasi`, `Berhasil dikenali`,
  `Sudah tercatat`, `Wajah belum dikenali`, `Perlu posisi lebih jelas`,
  `Lebih dari satu wajah`, `Kamera bermasalah`, `Gangguan jaringan`, `Sesi belum aktif`).
- Tes: `scripts/test_biometrics_ui.mjs` (`deriveKioskState maps kiosk lifecycle and
  recognition results to explicit states`).

### 3.5 Filter UI Admin (P1)

- `apps/web/app/lib/validation/master.ts`: `enrolledFilter` (Zod) pada `studentFilterSchema`.
- `apps/web/app/pages/admin/students/index.vue`: filter "Semua wajah / Wajah terdaftar /
  Belum terdaftar" + kolom `Wajah` (badge) pada DataTable.
- `apps/web/app/pages/admin/biometrics.vue`: filter status wajah pada toolbar.
- `apps/web/app/types/master.ts`: `Student.face_enrolled?: boolean`.
- Tes: `scripts/test_form_validation.mjs` (`enrolled` values valid/invalid).

### 3.6 Kontrak API & Deployment

- `docs/openapi/master-data.json`: `face_enrolled` pada `StudentList`, parameter `enrolled`
  pada `GET /students`.
- `docs/openapi/biometrics.json`: parameter `enrolled` pada `GET /biometrics/students`.
- `docs/openapi/reports.json`: `operations` object pada `Dashboard`.
- `deploy/compose.application.yml`: `AI_OPERATION_MODE` untuk service `ai`.
- `deploy/.env.example`: dokumentasi `AI_OPERATION_MODE=provisional`.
- `05-API.md`: route matrix diperbarui (`enrolled` filter, `operations` summary).

---

## 4. File Penting yang Berubah

**AI service (Python)**
- `services/ai/app/pipeline.py`, `services/ai/app/main.py`
- `services/ai/tests/test_recognition_service.py`, `services/ai/tests/test_approval.py`

**Backend (Go)**
- `services/api/internal/masterdata/students.go`, `services/api/internal/masterdata/biometrics.go`
- `services/api/internal/attendance/reports.go`
- `services/api/internal/platform/aiclient/client.go`
- `services/api/internal/masterdata/biometrics_integration_test.go`, `services/api/internal/attendance/integration_test.go`

**Frontend (Nuxt 4 / Vue 3 / shadcn-vue)**
- `apps/web/app/types/master.ts`, `apps/web/app/types/biometrics.ts`, `apps/web/app/types/reports.ts`
- `apps/web/app/lib/validation/master.ts`, `apps/web/app/lib/biometrics.ts`
- `apps/web/app/pages/admin/index.vue`, `apps/web/app/pages/admin/students/index.vue`, `apps/web/app/pages/admin/biometrics.vue`
- `apps/web/app/pages/absensi.vue`

**Tes**
- `scripts/test_form_validation.mjs`, `scripts/test_biometrics_ui.mjs`, `scripts/test_navigation_login.mjs`

**Kontrak & Deployment**
- `docs/openapi/master-data.json`, `docs/openapi/biometrics.json`, `docs/openapi/reports.json`
- `deploy/compose.application.yml`, `deploy/.env.example`
- `05-API.md`, `docs/UPGRADE-V2-PLAN.md`, `docs/UPGRADE-V2-REPORT.md`

---

## 5. Database Migrations

**Tidak ada migration baru yang diperlukan untuk perubahan ini.**
Filter `enrolled` dan `face_enrolled` dilayani dari relasi `face_templates`
(`school_id`,`student_id`) yang sudah ada dan terindeks; ringkasan `operations`
memakai `attendance_sessions` dan `camera_devices` yang juga sudah terindeks.
Tidak ada schema yang diubah, tidak ada data produksi yang disentuh.

---

## 6. API Changes

| Endpoint | Perubahan | Kompatibilitas |
|---|---|---|
| `GET /api/v1/students` | + query `enrolled` (`true`/`false`/kosong), + field `face_enrolled` | Backward compatible (opsional) |
| `GET /api/v1/biometrics/students` | + query `enrolled` (`true`/`false`/kosong); field output tetap 6 minimal | Backward compatible |
| `GET /api/v1/dashboard/summary` | + objek `operations` | Backward compatible (opsional) |
| AI `/internal/v1/health` & recognition result | + `operation_mode` | Backward compatible |

Nilai `enrolled` selain `""`, `"true"`, `"false"` menghasilkan `400 VALIDATION_ERROR`.

---

## 7. Security Improvements

- **Fail-closed AI gate**: mode `approved` memerlukan artefak laporan evaluasi dengan hash
  cocok (`load_approval`), konfigurasi engine cocok (threshold/margin), model checksum cocok,
  liveness threshold cocok, dan review manusia (`automatic_attendance_approved: true`).
  Semua mismatch → `None` → attendance otomatis diblokir.
- **Privasi selector biometrik**: `/biometrics/students` tetap mengembalikan 6 field minimal
  (tidak mengekspos `face_enrolled` berlebih; filter bekerja tanpa memperluas metadata).
- **Audit tidak membocorkan data sensitif**: assertion `sensitiveAudit` pada tes integrasi
  memastikan `ciphertext`, `PRIVATE-CANDIDATE`, dan `frames` tidak muncul di `audit_logs`.
- **Device revocation & gallery revision** tetap berlaku; template dari siswa yang dicabut
  tidak tersedia untuk matching (diuji oleh tes integrasi existing).

---

## 8. AI Improvements

- Operation mode gate dengan 4 mode dan fail-closed.
- `operation_mode` terekspos pada health/status untuk observability operator.
- Evaluasi approval (`services/ai/app/approval.py`) tetap menjadi satu-satunya jalan menuju
  `approved`; tidak ada approval palsu/otomatis yang dibuat.

---

## 9. UI/UX Improvements

- Dashboard admin: kartu **Sesi absensi berjalan**, **Perangkat kamera** (online/total),
  **Layanan pengenalan wajah** (mode + kesiapan), tabel **Aktivitas absensi terbaru** dengan
  empty state shadcn-vue (`Empty`), badge status semantic.
- Siswa: filter status wajah + kolom `Wajah` (badge `Terdaftar`/`Belum`) — memudahkan
  petugas menemukan siswa yang belum diregistrasi.
- Biometrik: filter status wajah pada toolbar.
- Kiosk `/absensi`: badge status proses eksplisit (12 state), tetap tanpa input manual,
  tetap tidak menampilkan nama sebelum backend mengonfirmasi.
- **Bahasa antarmuka dibersihkan dari istilah teknis** (permintaan pemilik produk + aturan
  blueprint): teks `Pipeline siap` / `Menunggu model AI` / nilai mentah mode operasi
  diganti kalimat sederhana (`recognitionStatusText` di `apps/web/app/lib/status.ts`),
  `Perlu review` → `Perlu ditinjau` (UI + label CSV Go), `Daftar siswa sesi` → `Peserta sesi`,
  `buka modal wajah` → `pendaftaran wajah`. **Skor kemiripan mentah dihapus dari layar
  kiosk publik** dan dari badge overlay kamera (sesuai `02-ARCHITECTURE.md` 6.10 &
  `06-SECURITY.md` 5 yang melarang skor mentah di layar publik; gap tercatat di
  `docs/UPGRADE-V2-PLAN.md`). Logika retensi skor internal tetap dan tetap teruji unit test;
  assert E2E `kiosk-feedback.spec.ts` diubah agar memverifikasi skor TIDAK tampil
  (perilaku yang diminta), bukan lagi menampilkan skor.

---

## 10. Test Matrix & Hasil Aktual

Eksekusi lokal pada lingkungan pengembangan (PostgreSQL tersedia):

| Test suite | Perintah | Hasil |
|---|---|---|
| Frontend lint | `npm --prefix apps/web run lint` | PASS |
| Frontend format | `npm --prefix apps/web run format:check` | PASS (bagian `make check`) |
| Frontend typecheck | `npm --prefix apps/web run typecheck` | PASS |
| Go vet + gofmt | `go vet ./...` + `gofmt -l` | PASS |
| Go tests (race) | `go test -race ./internal/auth ./internal/masterdata ./internal/attendance` | PASS (3 paket) |
| Python lint/format | `ruff check .` + `ruff format --check .` | PASS |
| Python tests | `pytest -q` (services/ai) | **88 passed** |
| Python script tests | `python3 -m unittest discover -s scripts` | **6 tests OK** |
| Node script tests | `node --test scripts/test_*.mjs` | **70 passed** |
| Browser smoke (Chrome nyata, login → dashboard) | verifikasi salinan UI & nol istilah teknis | PASS |
| Build semua service | `make build` | PASS |
| Compose validation | `make compose-check` | PASS |
| **Total `make check`** | lint + test | **PASS** |

Catatan: Playwright E2E (`apps/web/tests/e2e/security-ui.spec.ts`), migration job,
govulncheck, gitleaks, pip-audit, dan npm audit dijalankan oleh CI (`.github/workflows/ci.yml`)
dan tidak dieksekusi ulang penuh pada lingkungan lokal ini — lihat **Remaining Risks**.

---

## 11. Performance Benchmark

**NOT VALIDATED** — benchmark 500 identitas, CPU inference, P95/P99 latency, FPIR/FNIR,
dan uji serangan (print/replay) memerlukan dataset kalibrasi + perangkat kamera nyata
yang tidak tersedia pada lingkungan pengembangan ini. Angka benchmark tidak diklaim.
Target P95 kamera-ke-hasil < 2 detik tetap menjadi sasaran eksperimen, bukan klaim.

---

## 12. Remaining Risks & Blockers

1. **Kelayakan produksi biometrik — NEEDS HUMAN APPROVAL**: keberhasilan automated tests
   bukan berarti sistem aman untuk produksi; perlu validasi dataset, hardware, hukum
   (persetujuan wali), dan review manusia sebelum `AI_OPERATION_MODE=approved`.
2. **Biometric governance/consent lifecycle**: mekanisme persetujuan wali yang dicatat
   sebelumnya telah direformasi; dokumentasi dasar pemrosesan, retensi, dan penghapusan
   perlu ditinjau ulang oleh penanggung jawab hukum sekolah (di luar scope kode).
3. **Benchmark & liveness evaluation**: BLOCKED pada dataset/perangkat.
4. **E2E Playwright pada hardware kamera nyata**: NOT TESTED pada lingkungan lokal ini
   (dijalankan CI untuk regresi cross-origin).
5. **Coolify staging readiness**: compose tervalidasi (`make compose-check`), tetapi
   deployment aktual ke Coolify **belum diuji** — jangan mengklaim sukses sebelum
   diterapkan pada Coolify sebenarnya.

---

## 13. Operational Requirements

- Set `AI_OPERATION_MODE` secara eksplisit di Coolify untuk service `ai`:
  - `disabled` / `shadow` / `provisional` untuk uji coba.
  - `approved` hanya setelah laporan evaluasi independen ditinjau manusia, dengan
    `AI_RECOGNITION_APPROVAL_PATH` dan `AI_RECOGNITION_APPROVAL_SHA256` yang benar.
- Pastikan `AI_TEMPLATE_KEY_BASE64` (32 byte acak base64) dan `AI_SERVICE_TOKEN` diganti
  dengan rahasia asli (jangan pakai nilai contoh).
- Pastikan PostgreSQL hanya dapat diakses secara privat (jangan terekspos ke internet).
- Migration job (`migrate`) berjalan terpisah dengan user migrator least-privilege.

---

## 14. Release Readiness

| Aspek | Status |
|---|---|
| Functional (CRUD, kelas, koreksi, laporan, role) | DONE (regresi lulus) |
| Attendance integrity (anti-duplikat, idempotent) | DONE (diuji race) |
| AI gate fail-closed | DONE |
| Enrollment filter & flag | DONE |
| Dashboard operational summary | DONE |
| Kiosk state eksplisit | DONE |
| OpenAPI sinkron | DONE |
| Deployment config | DONE (belum diterapkan ke Coolify) |
| Benchmark/evaluasi produksi | NEEDS HUMAN APPROVAL |
| E2E hardware nyata | NOT TESTED |

**Kesimpulan**: kode siap untuk staging/pilot dengan `AI_OPERATION_MODE` selain `approved`.
Pengaktifan `approved` untuk produksi memerlukan persetujuan manusia dan evaluasi
independen yang belum tersedia pada iterasi ini.
