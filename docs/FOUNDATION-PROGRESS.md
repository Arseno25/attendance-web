# Foundation — pelaksanaan WP-01

Rencana induk: `07-IMPLEMENTATION-PLAN.md`. Scope yang disetujui: fondasi awal seluruh stack, bukan implementasi seluruh FR.

## Keputusan pelaksanaan

- Menggunakan direktori project yang masih kosong dari kode dan branch `foundation`; belum ada branch existing yang perlu diisolasi.
- npm digunakan karena tersedia; pnpm tidak ditambahkan sebagai package manager kedua.
- Go memakai `net/http` standard library; pgx untuk DB, goose CLI untuk migrations. Tidak menambah HTTP framework.
- Python memakai uv, FastAPI CLI dan ONNX Runtime CPU. Model/liveness belum dipilih; recognition harus fail-closed.
- Database baseline hanya schools untuk membuktikan migration, constraint dan koneksi. Tabel domain lengkap masuk WP-02 dan seterusnya.
- Compose production hanya web/api/ai. Compose PostgreSQL lokal terpisah untuk development/test.

## Checklist

- [x] Baca seluruh spesifikasi dan rencana induk pada sesi sebelumnya.
- [x] Verifikasi dokumentasi resmi melalui Context7 dan versi runtime/CLI.
- [x] Scaffold resmi Nuxt/shadcn, Go modules, Python uv; lock dependencies.
- [x] API health, request ID, CORS, config validation, shutdown dan tests.
- [x] Internal AI health/auth, runtime check, recognition disabled dan tests.
- [x] PostgreSQL migration/seed dummy dan integration checks.
- [x] Dockerfiles, Compose, CI, README, environment templates.
- [x] Build/typecheck/lint/tests, runtime smoke, container review.

## Bukti verifikasi

- `make check`: ESLint, Nuxt typecheck, Go vet/gofmt, Ruff, Go race tests, 6 pytest cases dan 1 bootstrap test lulus.
- `make build`: Nuxt production build, Go binary dan import ONNX CPU lulus.
- `make db-up`: Compose PostgreSQL lokal berhasil start dan healthy. `make migrate db-test`: migration dan constraint check lulus.
- Optional dummy seed lalu `make db-test` juga lulus; fixture test menggunakan UUID transaction-local dan rollback.
- Ketiga Dockerfiles berhasil dibangun menggunakan Podman/Docker-compatible CLI. Build awal API/AI terkena disk penuh; hanya intermediate image dari build task ini dibersihkan, cache image dikurangi dan rebuild lulus.
- Runtime containers: web 200 pada localhost:3001; API+DB readiness 200; CORS origin tidak sah 403; AI live 200/ready 503; token invalid 401; recognition token valid tetap 503 tanpa kandidat.
- Browser: tombol Periksa kembali menghasilkan “Layanan terhubung”. Kamera tidak diminta/diaktifkan.
- govulncheck dan pip-audit runtime dependencies lulus; staged secret scan Gitleaks tidak menemukan leaks. npm masih 19 temuan upstream (14 high, 5 moderate), tanpa critical setelah override; gate rilis mengikuti STACK-SETUP.
- CI workflow sudah ditulis, commands-nya diuji lokal; GitHub-hosted run belum dilakukan karena belum ada remote.

## Review dan keputusan akhir

Review independen membaca staged changes tanpa mengubah workspace. Tidak menemukan critical issue.

- Fixed: seed/test UUID collision — direproduksi sebagai duplicate key, diperbaiki memakai generated UUID dalam transaksi; seeded database check RED→GREEN.
- Fixed: exception AI 405 diberi code NOT_FOUND dan kehilangan Allow header — ditambahkan regression test, handler mempertahankan headers dan METHOD_NOT_ALLOWED; pytest RED→GREEN 6/6.
- Keputusan: TypeScript 6.0.3 dipin karena 7.0.2 belum kompatibel dengan compiler API Vue; upgrade setelah tooling mendukungnya, bukan dengan mematikan typecheck.
- Keputusan: web lokal memakai 3001 karena 3000 dipakai aplikasi lain; WEB_ORIGIN dan NUXT_PORT pada ignored local env disesuaikan. Default template tetap 3000.
- Deferred: fitur bisnis WP-02 onward, model/liveness/evaluasi WP-06, dan production network/TLS/roles/restore/image hardening WP-11. Batas ini memang berada di luar scope fondasi, bukan fitur yang diklaim selesai.

## Batasan review

Tidak ada login/CRUD/attendance endpoint yang dinyatakan selesai. Tidak ada data siswa asli, image/model biometrik, deployment Coolify atau secret production.
