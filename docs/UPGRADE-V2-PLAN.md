# Upgrade V2.0 — Baseline Audit & Implementation Plan

Tanggal audit: 17 Oktober 2026  
Branch: `main` (`dc34695`)  
Target: School Face Attendance System V2.0 (Nuxt 4 + Go modular monolith + Python FastAPI/ONNX Runtime + PostgreSQL 18.6)

---

## 1. Existing Implementation (Verified Baseline)

Berdasarkan inspeksi source code aktual (`apps/web`, `services/api`, `services/ai`, `deploy/`, `migrations 00001..00016`) dan eksekusi baseline test suite:

- **Authentication & Dynamic RBAC (`services/api/internal/auth`)**:
  - Argon2id password hashing, server-side session (`HttpOnly`, `SameSite=Lax`, `__Host-` prefix pada HTTPS), CSRF token terikat sesi, validasi `Origin` ketat, dan trusted proxy CIDR parsing (`TRUSTED_PROXY_CIDRS`).
  - Dynamic roles (`roles`, `role_permissions`) dengan scope `school` atau `assigned_classes` (`user_class_assignments`).
  - Perubahan role/status/akun mencabut `user_sessions` dan `device_sessions` terkait.
- **Master Data & Academic History (`services/api/internal/masterdata`)**:
  - Tahun ajaran, kelas, siswa (`UNIQUE(school_id, nis)`), histori penempatan non-overlapping (`student_enrollments`), penugasan kelas pengguna, serta import CSV dry-run + atomic commit (`import_batches`).
- **Biometrics & Face Template Security (`services/api/internal/masterdata/biometrics.go`, `services/ai/app/pipeline.py`)**:
  - Enkripsi AES-256-GCM per template dengan 12-byte random nonce dan AAD terikat `(school_id, student_id, consent_id, model_sha256, key_id)`.
  - Key disimpan di luar DB (`AI_TEMPLATE_KEY_BASE64`, `AI_TEMPLATE_KEY_ID`).
  - Pendaftaran 3–5 frame JPEG dengan pemeriksaan ukuran, kualitas (Laplacian blur + minimum face ratio), konsistensi intra-enrollment, dan deteksi duplikasi lintas siswa dalam sekolah.
  - Revisi galeri (`gallery_state`) otomatis naik melalui trigger DB saat template berubah/dihapus; nonaktifasi siswa memicu penghapusan template otomatis (`purge_ineligible_templates`).
  - Catatan tata kelola: sesuai keputusan pemilik proyek pada migrasi `00011_retire_face_consent.sql`, tabel `guardians`, `biometric_consents`, dan `biometric_tombstones` dipertahankan untuk kompatibilitas AAD historis, sementara pendaftaran operasional disederhanakan pada kelayakan siswa aktif dan otorisasi petugas.
- **AI Recognition Pipeline (`services/ai/app/face_engine.py`, `liveness.py`, `recognition.py`)**:
  - Model YuNet (deteksi), SFace (128-d embedding), dan MiniFASNet v2 (PAD/liveness) diverifikasi ukuran + SHA-256 terhadap `models/manifest.json`.
  - Urutan pipeline: decode & ukuran JPEG -> face detection -> single-face & quality gate -> IoU tracking -> feature extraction -> exact 1:N cosine matching (threshold + runner-up margin) -> multi-frame temporal confirmation -> MiniFASNet PAD -> server revalidation.
  - Anti-replay: endpoint `POST /api/v1/kiosk/capture-challenge` menerbitkan `event_id` sekali pakai (TTL 10 detik) terikat sesi perangkat dan menyimpan hash JPEG selama 24 jam; layanan AI juga menolak frame identik berulang pada buffer per perangkat.
- **Attendance, Kiosk & Reporting (`services/api/internal/attendance`)**:
  - Sesi absensi, kalender sekolah + sinkronisasi libur nasional, check-in, check-out otomatis/manual, koreksi dengan larangan self-approval, rekonsiliasi alfa setelah sesi ditutup, laporan harian/rincian + ekspor CSV dengan proteksi formula injection, dan notifikasi invalidasi WebSocket (`/api/v1/ws/attendance`).
  - Kiosk `/absensi` berjalan tanpa sidebar admin, menghentikan sesi operator saat pairing, mendukung persistent device cookie (30 hari atau batas akhir penugasan kelas), dan hanya menampilkan identitas setelah transaksi absensi diverifikasi server.
- **Baseline Test Suite Execution**:
  - `make check`: **LULUS** (ESLint, Prettier, Nuxt typecheck, `go vet`, `gofmt`, `ruff`, `go test -race ./...`, 87 pytest AI, 6 unittest Python, 68 Node UI/contract tests).
  - `make db-test`: **LULUS** (`foundation.sql` + Go race integration tests untuk `auth`, `masterdata`, dan `attendance` terhadap PostgreSQL 18.6).
  - `make compose-check`: **LULUS**.

---

## 2. Gap Analysis (Missing Features, Risks & Inconsistencies)

### 2.1 Security & Biometric Governance / AI Operation Mode (P0)
- **GAP-V2-01 (AI Operation Mode Gate — Phase 2.7)**:
  - Saat ini `services/ai` langsung mengaktifkan pengenalan (`recognition_enabled = True`) selama model PAD dapat dimuat, meskipun `AI_RECOGNITION_APPROVAL_PATH` kosong.
  - Belum ada kontrol eksplisit mode operasi `AI_OPERATION_MODE` (`disabled`, `shadow`, `approved`, serta `development` / default yang kompatibel untuk pengujian lokal) yang dapat:
    1. Mematikan absensi biometrik (`disabled`).
    2. Menjalankan evaluasi pengenalan secara `shadow` tanpa mencatat kehadiran otomatis atau membocorkan identitas siswa ke layar publik.
    3. Memaksa fail-closed pada mode `approved` jika laporan evaluasi independen (`load_approval`) tidak valid atau tidak cocok dengan checksum model/konfigurasi runtime.
- **GAP-V2-02 (Public Kiosk Privacy — Phase 5)**:
  - Halaman `/absensi` saat ini menampilkan **Skor kemiripan** (`similarity`) secara eksplisit pada kartu hasil kiosk publik. Blueprint (`02-ARCHITECTURE.md` bagian 6.10 & `06-SECURITY.md` bagian 5) dan requirement Phase 5 melarang penampilan skor kemiripan mentah pada layar kiosk publik dan menuntut pembatasan PII di layar publik.

### 2.2 Admin Panel & Student / Device Monitoring (P1)
- **GAP-V2-03 (Filter Status Enrollment Wajah pada Halaman Siswa — Phase 4.3)**:
  - `GET /api/v1/students` dan halaman `/admin/students` sudah memiliki pencarian nama/NIS, filter kelas, dan filter status siswa, tetapi belum menyediakan informasi status pendaftaran wajah (`face_enrolled`) maupun filter status enrollment wajah (`enrolled` / `not_enrolled`).
- **GAP-V2-04 (Filter Status Enrollment pada Halaman Biometrik — Phase 4.4)**:
  - `GET /api/v1/biometrics/students` dan `/admin/biometrics` belum menampilkan badge status pendaftaran wajah secara eksplisit di kolom tabel dan belum memiliki filter `enrolled` / `not_enrolled` untuk memudahkan petugas mencari siswa yang belum diregistrasi.
- **GAP-V2-05 (Dashboard Camera & Active Session Summary — Phase 4.2 & 4.5)**:
  - `GET /api/v1/dashboard/summary` dan `/admin` menampilkan metrik kehadiran, grafik tren, dan catatan terbaru, tetapi belum menampilkan ringkasan sesi yang sedang berjalan (`open_sessions`) dan ringkasan kesehatan perangkat kamera (`devices_summary`: total aktif, online dalam 5 menit terakhir, dicabut) serta status kesiapan AI.

### 2.3 Kiosk State Feedback & Documentation Alignment (P1/P2)
- **GAP-V2-06 (Indikator State Kiosk Eksplisit — Phase 5)**:
  - `/absensi` sudah menangani alur kamera, namun belum memetakan state operasional secara terstruktur (`initializing`, `camera_ready`, `detecting`, `processing`, `recognized`, `already_recorded`, `unknown`, `inconclusive`, `multiple_faces`, `camera_error`, `network_error`, `session_inactive`) ke badge/status indikator yang jelas bagi siswa dan operator.
- **GAP-V2-07 (Sinkronisasi Dokumentasi & OpenAPI — Phase 3.3 & 10)**:
  - `README.md`, `05-API.md`, `docs/openapi/*.json`, dan `docs/STAGING-RUNBOOK.md` perlu disinkronkan dengan route `/` -> `/login`, mode operasi AI (`AI_OPERATION_MODE`), filter `face_enrolled`, serta tambahan ringkasan dashboard.

---

## 3. Database Migration & Dependency Assessment

- **Database Migrations**:
  - Schema existing (`00001` s/d `00016`) sudah mencakup tabel, indeks, constraint unik `(school_id, nis)`, `(session_id, student_id)`, `kiosk_capture_challenges`, `gallery_state`, dan `device_sessions`.
  - Fitur filter `face_enrolled` dan ringkasan perangkat/sesi pada dashboard dapat dilayani langsung dari relasi `face_templates`, `camera_devices`, dan `attendance_sessions` yang sudah terindeks tanpa perubahan destruktif pada schema. Tidak diperlukan migration baru yang merusak kompatibilitas.
- **Dependency Compatibility**:
  - Stack saat ini menggunakan versi stabil terkunci (`nuxt 4.6.1`, `vue 3.5.44`, `go 1.27.2`, `python 3.14.8`, `postgresql 18.6`).
  - `pip-audit` dan `govulncheck` bersih dari kerentanan runtime yang dapat dipanggil.
  - `deploy/dependency-exceptions.json` masih berlaku hingga `2026-11-15` untuk 3 advisory build-time npm yang tidak masuk ke bundle runtime `.output/server`. Tidak ada dependency baru yang perlu ditambahkan.

---

## 4. Prioritized Work Plan & Acceptance Criteria

### P0 — Security, AI Operation Mode Gate & Kiosk Privacy
1. **AI Operation Mode (`AI_OPERATION_MODE`)**:
   - Tambahkan dukungan `AI_OPERATION_MODE` pada `services/ai` (`disabled`, `shadow`, `approved`, `provisional`/`development`).
   - Mode `disabled`: `/internal/v1/recognize` dan `/internal/v1/status` melaporkan `recognition_enabled=False`, `block_reason="RECOGNITION_DISABLED"`, dan tidak mengembalikan keputusan `accepted`.
   - Mode `shadow`: pipeline tetap menjalankan deteksi, pencocokan, dan PAD untuk observasi/evaluasi internal, tetapi mengembalikan `recognition_enabled=False` dan `decision="inconclusive"` (`block_reason="SHADOW_MODE"`) sehingga Go API tidak pernah menulis attendance atau menampilkan nama siswa di kiosk.
   - Mode `approved`: wajib memiliki `evaluation` berstatus `"reviewed"` dari `load_approval(...)`. Jika tidak valid/hilang, fail-closed (`recognition_enabled=False`, `block_reason="APPROVAL_REQUIRED"`).
   - Mode default (`provisional`): mempertahankan perilaku operasional saat ini agar tidak memutus alur kamera Fantech lokal dan tes regresi yang sudah ada, sembari tetap mewajibkan single face + quality + temporal stability + PAD + server revalidation.
   - **Acceptance Criteria**: Unit test di `services/ai/tests/` membuktikan `disabled`, `shadow`, dan `approved` (tanpa/dengan approval valid) bersifat fail-closed terhadap pencatatan otomatis.
2. **Kiosk Public Privacy Hardening**:
   - Hapus tampilan angka `similarity` (skor kemiripan) dari layar publik `/absensi` dan dari badge kotak deteksi `FaceCamera.vue` / `biometrics.ts`.
   - **Acceptance Criteria**: Kiosk hanya menampilkan nama siswa, status kehadiran, dan jam tercatat setelah konfirmasi server; skor kemiripan mentah tidak dirender di UI publik.

### P1 — Backend API, Admin Panel & Kiosk UX Polish
1. **Student & Biometric Enrollment Status Filter**:
   - Perbarui `GET /api/v1/students` dan `GET /api/v1/biometrics/students` agar menyertakan boolean `face_enrolled` dan mendukung query filter `enrolled=true|false`.
   - Tambahkan dropdown filter status wajah (`Semua wajah`, `Sudah terdaftar`, `Belum terdaftar`) dan badge status wajah pada `/admin/students` dan `/admin/biometrics`.
   - **Acceptance Criteria**: Filter bekerja secara server-side dengan validasi input ketat, tercermin di UI dan diuji melalui tes backend & frontend.
2. **Dashboard Operational Summary (Sessions, Devices & AI Status)**:
   - Perbarui `GET /api/v1/dashboard/summary` agar menyertakan ringkasan `operations`: jumlah sesi terbuka hari ini (`open_sessions`), ringkasan perangkat (`total_devices`, `online_devices`, `revoked_devices`), serta status kesiapan layanan wajah (`ai_status`).
   - Tampilkan kartu ringkasan operasional (Sesi aktif, Kamera online, Status pengenalan wajah, dan Aktivitas absensi terbaru) pada `/admin`.
   - **Acceptance Criteria**: Dashboard merender data nyata dari API tanpa dummy stats dan tetap responsif pada layar 320px–1440px.
3. **Kiosk State Machine Badge**:
   - Tambahkan pemetaan state eksplisit (`initializing`, `camera_ready`, `detecting`, `processing`, `recognized`, `already_recorded`, `unknown`, `inconclusive`, `multiple_faces`, `camera_error`, `network_error`, `session_inactive`) pada `/absensi` agar operator/siswa melihat status perangkat yang konsisten.
   - **Acceptance Criteria**: State kiosk teruji pada tes regresi UI.

### P2 — Documentation, OpenAPI & Final Verification Report
1. Perbarui `docs/openapi/*.json`, `README.md`, `05-API.md`, dan `docs/STAGING-RUNBOOK.md`.
2. Buat `docs/UPGRADE-V2-REPORT.md` berisi ringkasan perubahan, matriks pengujian nyata, hasil benchmark CPU 500 identitas, dan status kesiapan rilis.
