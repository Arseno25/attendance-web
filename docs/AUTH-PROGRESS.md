# WP-02 — Auth, pengguna, izin dan audit

Base: `7108ead`; acuan: `07-IMPLEMENTATION-PLAN.md` WP-02 dan UI-01, PRD FR-01/14/16, API dan security matrix. User meminta melanjutkan pembangunan setelah foundation.

## Desain pelaksanaan

- Go tetap pemilik auth; Nuxt tidak membuat auth database kedua atau menyimpan token di localStorage.
- Password Argon2id dengan salt acak, 64 MiB/3 pass/2 thread; input dibatasi, dua hashing concurrency maksimum.
- Sesi opaque 256-bit, hanya SHA-256 token disimpan di DB. TTL absolut 8 jam, idle 30 menit; identity/peran/eligibility dibaca ulang dari DB.
- Cookie host-only, HttpOnly, SameSite Lax; Secure dan prefix __Host pada konfigurasi HTTPS. HTTP cookie hanya untuk development localhost.
- CSRF per sesi diturunkan dari secret session token, dikirim lewat JSON ke client yang sah dan wajib pada operasi tulis. Login memerlukan exact Origin dan JSON; logout/user writes membutuhkan Origin + CSRF.
- Satu sekolah dikonfigurasi melalui SCHOOL_ID. Guru tanpa assignment tidak mendapat school-wide data. Users/audit hanya super admin.
- User create/update, role/grants/status, session revocation dan audit dilakukan atomik. Tidak boleh menonaktifkan/mengurangi akses sendiri atau super admin terakhir.
- Bootstrap admin via CLI password stdin, tanpa default password; hanya berhasil bila sekolah belum memiliki pengguna.
- Login/admin pages memakai client rendering karena cookie API host-only pada domain API terpisah tidak tersedia pada SSR web. Semua API tetap memeriksa akses server-side.
- Rate limit login bounded per-process/IP/account; pindah ke shared store sebelum multi-instance. Proxy headers tidak dipercaya untuk penentuan IP pada MVP.

## Pekerjaan dan bukti

- [x] Migration users/sessions/grants/audit dan bootstrap CLI.
- [x] Unit tests hashing, policy, limiter; API integration dengan PostgreSQL nyata.
- [x] Login/logout/me, user management, audit dan security checks.
- [x] Nuxt login, admin shell, users/audit UI, expired/forbidden handling.
- [x] Config/docs/CI integration, build/typecheck/lint/audits lokal.
- [x] Runtime smoke dan review independen.

Verifikasi 2026-10-12:

- `make check build compose-check` exit 0; final `make check` sesudah tambahan regression tests juga exit 0. Go race tests, lint/typecheck web, enam pytest AI dan satu test setup env lulus.
- `make migrate db-test` menerapkan migration 2; integration PostgreSQL menggunakan schema terisolasi dan cleanup. Lifecycle login/logout/rotation, idle 30 menit, TTL absolut, sekolah/akun nonaktif, CSRF, scope sekolah, role restrictions, bootstrap sekali, duplikat email, immutable audit dan token hash terbukti. Policy kelas diuji unit; assignment kelas nyata menunggu WP-03.
- Reviewer independen memeriksa transaksi, revocation, scope, CSRF, frontend dan schema. Dua temuan UUID direproduksi (403 pada uppercase SCHOOL_ID dan 200 pada self-email update via uppercase ID), lalu diperbaiki dengan canonical school ID dan pembandingan target DB ID. Regression keduanya kini lulus.
- Image `localhost/attendance-web:auth` dan `localhost/attendance-api:auth` berhasil dibangun. Runtime container: login/me/users/audit/readiness 200, CSRF salah 403, logout 200 dan sesi sesudah logout 401. Container preview project berjalan pada port 3001/8080; AI foundation tetap di 8001.
- Browser memverifikasi login berhasil/gagal, logout, redirect halaman admin sesudah logout, daftar pengguna, pencarian kosong dan audit. Screenshot lokal berada di `.tools/auth-preview.png`.
- govulncheck: 0 vulnerability reachable/imported; 1 advisory pada module yang tidak dipanggil. npm audit: 19 upstream findings (14 high, 5 moderate, 0 critical), sama dengan WP-01. Audit belum bersih; lihat STACK-SETUP untuk gate rilis.
- `gitleaks git --pre-commit --staged --redact .`: tidak ada secret pada perubahan yang akan di-commit. Credential lokal tetap ignored dengan mode `0600`.
- CI database job memakai target `db-test` yang kini menjalankan integration auth. Workflow GitHub belum dijalankan di remote; verifikasi di atas lokal.

## Dokumentasi setup

Context7 memverifikasi pgx transactions/Scan, Nuxt middleware/useState/fetch serta shadcn form components. Context7 tidak memiliki dokumentasi Go Argon2 yang cocok; fallback primer: https://pkg.go.dev/golang.org/x/crypto/argon2. `go get golang.org/x/crypto@latest` dan CLI shadcn yang dipin dipakai untuk install.

Scope belum mencakup siswa/kelas/absensi, assignment guru nyata, MFA/forgot-password/email, dan biometrik. Audit mencatat actor/action/target/time/request ID; changed-field before/after metadata, retention dan proteksi dari DB owner/TRUNCATE belum tersedia. Limiter perlu shared store serta proxy trust yang ditetapkan sebelum production multi-instance. Web/API production harus HTTPS pada site yang sama untuk cookie SameSite Lax.

Berikutnya: WP-03 — academic years, classes, teacher assignments, students dan placements; UI-02 dan pemeriksaan scope dengan data kelas nyata.
