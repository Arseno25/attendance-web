# School Face Attendance

Fondasi aplikasi absensi satu sekolah. Nuxt 4 untuk web admin/kiosk, Go untuk API dan transaksi, Python FastAPI/ONNX Runtime untuk service AI internal, serta PostgreSQL sebagai resource terpisah.

Status lokal: **core WP-01–05, perangkat/kiosk, P1 dan enrollment provisional**. Semua form menggunakan aturan Zod modular; panel menyediakan edit akun, tema terang/gelap/sistem, pagination bernomor dan aksi halaman di kanan atas. Model YuNet/SFace terpin menjalankan deteksi CPU dan pendaftaran template AES-GCM. `/absensi` memproses frame untuk kotak deteksi; nama hanya tampil setelah keputusan server diterima dan kehadiran terkonfirmasi. Akurasi/liveness **belum dievaluasi**, sehingga absensi wajah otomatis tetap tertutup. Lihat [perubahan UI, akun dan wajah](docs/UI-ACCOUNT-BIOMETRICS-PROGRESS.md), [status paket sebelumnya](docs/LOCAL-COMPLETION-PROGRESS.md) dan [runbook lokal](docs/LOCAL-RUNBOOK.md).

Panel admin memakai block resmi shadcn-vue `dashboard-01` dengan sidebar inset, menu akun, breadcrumb dan komponen shadcn untuk forms/tables. Halaman kiosk `/absensi` memakai route/layout mandiri di luar panel. Detail CLI, penyesuaian dan pembagian route ada di [desain panel](docs/PANEL-DESIGN.md).

## Mulai di lokal

Prasyarat: Node 26, npm, Go dengan toolchain auto-download, Python 3 untuk bootstrap, Docker Compose v2 (atau Podman dengan Compose provider), dan `psql` untuk pemeriksaan database. uv mengelola Python 3.14.8 dan dependency project; tidak perlu install package Python secara global.

```bash
make setup
make db-up
make migrate db-test
```

`make setup` menginstal CLI uv/goose yang dipin di `.tools`, memasang dependencies dari lockfile, dan membuat `.env` lokal berisi secret acak dengan permission `0600`. File existing tidak ditimpa. Jangan mengganti password DB yang sudah terinisialisasi hanya dengan mengedit `.env`; ubah password di PostgreSQL juga.

Jalankan setiap service development di terminal terpisah:

```bash
make dev-api
make dev-ai
make dev-web
```

Web development: `http://localhost:3001`. API: `http://127.0.0.1:8080/api/v1`. AI lokal: `http://127.0.0.1:8001/internal/v1`, terikat ke loopback. `localhost` dan `127.0.0.1` merupakan origin browser berbeda; sesuaikan `WEB_ORIGIN` jika membuka web menggunakan alamat lain.

Untuk menjalankan web dalam Compose pada loopback host port mulai dari 3001, otomatis lanjut ke port berikutnya bila bentrok:

```bash
make compose-web
```

Perintah menampilkan URL port yang dipilih; web tetap menggunakan port container 3000. Launcher mengaktifkan allowlist `localhost:3001–3010` khusus API Compose lokal. Default tetap mati dan tidak melonggarkan origin deployment.

Untuk development lokal, port web dan `WEB_ORIGIN` default adalah 3001. Jika port itu dipakai aplikasi lain, ubah `NUXT_PORT` dan `WEB_ORIGIN` dalam `.env` ke port yang sama, lalu restart API/web. Mode Compose memakai `make compose-web` untuk mencari port host mulai 3001.

Preview container web/API/AI hasil verifikasi saat ini masih berjalan pada port 3001/8080/8001. Sebelum menjalankan mode dev pada port tersebut, hentikan preview milik project ini:

```bash
docker stop attendance-foundation-web attendance-foundation-api attendance-foundation-ai
```

Database Compose tetap berjalan; perintah ini tidak menghapus volume atau data.

### Administrator pertama

Tidak ada password bawaan. Sesudah migration, build CLI dan buat super admin pertama di database yang belum memiliki pengguna untuk `SCHOOL_ID` tersebut:

```bash
make build
set -a
source .env
set +a
read -r -s -p 'Password admin (12–128 byte): ' attendance_admin_password
printf '%s' "$attendance_admin_password" | .tools/bootstrap \
  --email admin@school.test --name 'Admin Sekolah' --school-name 'Nama Sekolah'
unset attendance_admin_password
```

Password masuk lewat stdin, bukan argumen CLI atau log. Bootstrap berikutnya ditolak. Buka `/login`, lalu `/admin`; menu pengguna dan audit hanya tersedia bagi super admin. Untuk deployment, binary `bootstrap` juga tersedia di image API; jalankan dengan environment/secrets yang sama dan stdin privat.

Workspace ini sudah memiliki akun pengembangan, tersimpan hanya di `.tools/local-admin.json` (ignored, permission `0600`). Untuk membuat administrator pada instalasi development lokal yang masih kosong, jalankan `make build`, lalu `DEVELOPMENT_MODE=1 make dev-bootstrap`; perintah memeriksa `WEB_ORIGIN` dan `DATABASE_URL` loopback dan memakai password development `Password1!`. Kredensial ini hanya untuk development lokal, jangan gunakan di deployment atau akun nyata. Bootstrap normal di semua lingkungan tetap menerima password kuat melalui stdin. `SCHOOL_ID` wajib berupa UUID dan harus menunjuk sekolah yang akan dikelola; jangan menggantinya untuk berpindah sekolah tanpa memeriksa data.

Web dan API production harus menggunakan HTTPS pada host dengan **site yang sama**, misalnya `app.school.example` dan `api.school.example`, agar cookie SameSite Lax dapat dikirim pada request web ke API. Domain lintas site memerlukan desain auth berbeda. Exact origin web tetap dikonfigurasi melalui `WEB_ORIGIN`.

```bash
make check
make build
make compose-check
make migration-status
```

Seed sekolah dummy opsional, tanpa user/password default atau biometrik:

```bash
SEED_DEVELOPMENT=1 make seed
```

`make db-stop` menghentikan PostgreSQL tanpa menghapus volume. Hindari `down -v` pada database berisi data yang diperlukan.

## Struktur

```text
apps/web/                 Nuxt app, Tailwind, shadcn-vue, ESLint/TypeScript
services/api/             Go net/http, pgx, migrations dan SQL integration checks
services/ai/              FastAPI internal, ONNX CPU, pytest/ruff, uv.lock
deploy/                   Compose aplikasi dan database lokal yang terpisah
scripts/                  Bootstrap env lokal dan test-nya
.github/workflows/ci.yml  Lint/tests/build/migration, audits dan image builds
docs/                     Catatan setup, keputusan dan status fondasi
```

Dokumen kebutuhan tetap berada di root. Lihat [rencana induk](07-IMPLEMENTATION-PLAN.md), [setup stack](docs/STACK-SETUP.md), [progress fondasi](docs/FOUNDATION-PROGRESS.md), dan [progress auth](docs/AUTH-PROGRESS.md).

## Endpoint fondasi

| Endpoint | Perilaku |
|---|---|
| GET `/api/v1/health/live` | 200 bila Go API hidup |
| GET `/api/v1/health/ready` | 200 bila DB dapat di-ping, 503 bila tidak; bukan bukti seluruh schema/domain siap |
| POST `/api/v1/auth/login` | Login JSON dengan exact Origin; membuat cookie HttpOnly |
| GET `/api/v1/auth/me` | Profil, school scope, izin dan CSRF sesi aktif |
| PATCH `/api/v1/auth/profile` | Edit akun sendiri; perubahan email/password memerlukan password saat ini dan merotasi seluruh sesi |
| POST `/api/v1/auth/logout` | Origin + CSRF; mencabut sesi |
| GET/POST `/api/v1/users` | Super admin: pencarian/pagination dan pembuatan akun |
| PATCH `/api/v1/users/{id}` | Super admin: perubahan profil/akses dan pencabutan sesi terkait |
| GET `/api/v1/audit-logs` | Super admin: audit per sekolah, pagination |
| GET `/internal/v1/health/live` | 200 bila runtime CPU ONNX tersedia |
| GET `/internal/v1/health/ready` | 200 saat model dan kunci enrollment siap; recognition tetap false sampai evaluasi |
| GET `/api/v1/biometrics/students` | Seleksi siswa terbatas untuk petugas enroll/revoke |
| GET `/api/v1/students/{id}/face-status` | Status pendaftaran dan evaluasi tanpa template plaintext |
| POST `/api/v1/students/{id}/enrollments` | 3–5 JPEG; siswa aktif, kualitas dan konsistensi; template terenkripsi |
| POST `/api/v1/students/{id}/face-revoke` | Izin revoke + alasan; hapus template |
| POST `/api/v1/kiosk/recognize` | Cookie device + Origin/CSRF; kotak deteksi tanpa nama kandidat |
| POST `/internal/v1/recognize` | Token internal wajib; deteksi nyata, identitas otomatis masih tertutup |

Go mengembalikan error JSON, generated request ID, exact-origin CORS dan no-store headers. Log Go hanya metadata request, bukan body, URL query atau secret. Production AI mematikan access log Uvicorn; FastAPI dev CLI hanya dipakai untuk data dummy lokal. Kamera aktif hanya melalui tindakan petugas setelah memilih siswa atau pairing perangkat. Frame diproses tanpa menyimpan foto, dan automatic attendance masih tertutup.

## Deployment Coolify

1. Buat PostgreSQL standalone Database resource. Provision role runtime aplikasi dengan hak minimum untuk operasi aplikasi dan role migrator terpisah dengan hak DDL yang diperlukan migrasi Goose; jangan gunakan superuser sebagai kredensial runtime. Isi `DATABASE_URL` dan `MIGRATION_DATABASE_URL` secara terpisah di secret Coolify.
2. Deploy `deploy/compose.application.yml`. Selain `web`, `api`, dan `ai`, Compose menjalankan `init-models` serta `migrate` sekali-jalan. Build context relatif ke file Compose. Coolify harus memakai root project yang mencakup `apps/`, `services/`, dan `deploy/`.
3. Isi nilai yang diperlukan dari `deploy/.env.example` melalui secret/config Coolify. Tidak ada fallback kredensial production. Perhatikan SSL mode yang sesuai resource DB; jangan mengasumsikan setiap koneksi privat memakai TLS tanpa memverifikasinya.
4. Hubungkan Compose ke predefined destination network yang sama dengan DB, lalu verifikasi private DNS/port. Baca [networking resmi Coolify](https://github.com/coollabsio/coolify-docs/blob/main/content/docs/core/networking-in-coolify.mdx).
5. Model YuNet/SFace diunduh ke named volume `model_data` (project-scoped oleh Compose) oleh `init-models`; installer memeriksa ukuran dan SHA-256 manifest. `ai` menunggu init sukses dan memasang volume read-only. Host perlu akses keluar ke sumber model saat volume pertama kali dibuat. Volume bertahan pada redeploy dengan nama project Compose yang sama, tidak berbagi antar-project, tetapi hanya berada pada host itu dan bukan backup. Jika nama project berubah, Compose membuat volume baru dan init mengunduh model lagi; volume lama tidak otomatis dihapus. Buat backup dan uji restore sebelum pindah host. Jangan mount path model relatif checkout.
6. Service `migrate` menjalankan image API yang sama, Goose `/app/migrations up`, memakai hanya `MIGRATION_DATABASE_URL`. API menunggu `migrate` selesai sukses (`service_completed_successfully`); kegagalan migrasi harus menghentikan startup API. Ini berjalan saat Compose membuat ulang stack, bukan hanya saat file migrasi berubah. Verifikasi perilaku retry/reconciliation pada versi Coolify target sebelum rilis.
7. Setelah migrasi sukses dan API siap, buat administrator pertama secara manual satu kali. Dari shell operator, minta password tanpa echo dengan `read -r -s -p 'Password admin: ' ADMIN_PASSWORD`, lalu jalankan `printf '%s' "$ADMIN_PASSWORD" | docker exec -i <api-container-id> bootstrap --email admin@school.example --name 'Admin Sekolah' --school-name 'Nama Sekolah'`; sesudahnya `unset ADMIN_PASSWORD`. Ganti placeholder dengan ID container API aktif dari Coolify. Password hanya lewat stdin, tidak di command arguments, environment, Compose atau log. CLI menolak bootstrap kedua. Lanjut verifikasi login, origin, cookie Secure dan pengelolaan pengguna.
8. Verifikasi health, konfigurasi runtime, backup/restore database dan model. Health AI di Compose memakai liveness; readiness model/enrollment terpisah dari gate recognition dan bukan persetujuan biometrik produksi.

Konfigurasi Compose divalidasi lokal, tetapi belum dijalankan pada server Coolify. Sebelum trafik produksi, uji live bahwa named volume dan secret bekerja, API gagal mulai jika migrasi gagal, dan init model dapat mengisi volume kosong. Backup volume tetap diperlukan. Belum ada deployment ke akun/server Coolify pada tahap ini; domain, credential production, legal policy, model license dan pilot tetap mengikuti gate pada rencana induk.

## Model dan alur lokal

```bash
cd services/ai
../../.tools/uv run python -m app.models --directory ../../.tools/models
```

Installer memverifikasi ukuran dan SHA256 dari OpenCV Zoo sebelum memuat model. Directory model ignored dan license disimpan bersama model. CLI evaluator `python -m app.evaluation /private/authorized-holdout.json` merangkum trace anonim; tidak menghasilkan persetujuan otomatis.

Panel: `/admin/settings` untuk tema Terang/Gelap/Sistem dan identitas sekolah berizin; `/admin/account` untuk akun sendiri, `/admin/biometrics` untuk petugas enroll/revoke, `/admin/devices` untuk device, `/admin/students/import` untuk CSV, detail sesi untuk pulang/live. Pairing memakai token5menit sekali pakai dan cookie device10menit. Import preview15menit, maks500baris. Kontrak endpoint baru [lifecycle.json](docs/openapi/lifecycle.json). Tetap lokal sesuai instruksi; Coolify/pilot belum dijalankan.

## Audit dependency

CI menjalankan govulncheck dan pip-audit. npm audit menghasilkan artifact dan warning yang harus direview sebelum rilis; job tersebut tidak dinyatakan sebagai audit bersih. Temuan upstream yang belum memiliki patch kompatibel dicatat dalam [setup stack](docs/STACK-SETUP.md). Jangan menjalankan `npm audit fix --force` tanpa menilai perubahan major/downgrade yang ditawarkannya.
