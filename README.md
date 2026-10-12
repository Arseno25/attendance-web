# School Face Attendance

Fondasi aplikasi absensi satu sekolah. Nuxt 4 untuk web admin/kiosk, Go untuk API dan transaksi, Python FastAPI/ONNX Runtime untuk service AI internal, serta PostgreSQL sebagai resource terpisah.

Status: **WP-01 foundation**. Login, CRUD siswa, absensi, enrollment dan recognition produksi belum diimplementasikan. Halaman awal menunjukkan koneksi API/DB sebenarnya, tanpa data siswa demo yang diklaim nyata. AI sengaja mengembalikan `503` sampai model, liveness dan evaluasi WP-06 selesai.

## Mulai di lokal

Prasyarat: Node 26, npm, Go dengan toolchain auto-download, Python 3 untuk bootstrap, Docker Compose v2 (atau Podman dengan Compose provider), dan `psql` untuk pemeriksaan database. uv mengelola Python 3.14.8 dan dependency project; tidak perlu install package Python secara global.

```bash
make setup
make db-up
make migrate db-test
```

`make setup` menginstal CLI uv/goose yang dipin di `.tools`, memasang dependencies dari lockfile, dan membuat `.env` lokal berisi secret acak dengan permission `0600`. File existing tidak ditimpa. Jangan mengganti password DB yang sudah terinisialisasi hanya dengan mengedit `.env`; ubah password di PostgreSQL juga.

Jalankan setiap service di terminal terpisah:

```bash
make dev-api
make dev-ai
make dev-web
```

Web: `http://localhost:3000`. API: `http://127.0.0.1:8080/api/v1`. AI lokal: `http://127.0.0.1:8001/internal/v1`, terikat ke loopback. `localhost` dan `127.0.0.1` merupakan origin browser berbeda; sesuaikan `WEB_ORIGIN` jika membuka web menggunakan alamat lain.

Jika port 3000 dipakai aplikasi lain, ubah `NUXT_PORT` dan `WEB_ORIGIN` dalam `.env` ke port yang sama, lalu restart API/web. Pada verifikasi workspace ini web memakai port **3001** karena 3000 sudah digunakan.

Preview container web/API/AI hasil verifikasi saat ini masih berjalan pada port 3001/8080/8001. Sebelum menjalankan mode dev pada port tersebut, hentikan preview milik project ini:

```bash
docker stop attendance-foundation-web attendance-foundation-api attendance-foundation-ai
```

Database Compose tetap berjalan; perintah ini tidak menghapus volume atau data.

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

Dokumen kebutuhan tetap berada di root. Lihat [rencana induk](07-IMPLEMENTATION-PLAN.md), [setup stack](docs/STACK-SETUP.md), dan [progress](docs/FOUNDATION-PROGRESS.md).

## Endpoint fondasi

| Endpoint | Perilaku |
|---|---|
| GET `/api/v1/health/live` | 200 bila Go API hidup |
| GET `/api/v1/health/ready` | 200 bila DB dapat di-ping, 503 bila tidak; bukan bukti seluruh schema/domain siap |
| GET `/internal/v1/health/live` | 200 bila runtime CPU ONNX tersedia |
| GET `/internal/v1/health/ready` | 503 `MODEL_NOT_CONFIGURED` pada fondasi |
| POST `/internal/v1/recognize` | 401 tanpa token sah; 503 `RECOGNITION_DISABLED` dengan token sah |

Go mengembalikan error JSON, generated request ID, exact-origin CORS dan no-store headers. Log Go hanya metadata request, bukan body, URL query atau secret. Production AI mematikan access log Uvicorn; FastAPI dev CLI hanya dipakai untuk data dummy lokal. UI kamera belum diaktifkan.

## Deployment Coolify

1. Buat PostgreSQL standalone Database resource. Gunakan role aplikasi terbatas dan role migration terpisah; akun DB development bukan konfigurasi production.
2. Deploy `deploy/compose.application.yml` dengan tiga service `web`, `api`, `ai`. Build context relatif ke file Compose. Konfigurasi root project bila menggunakan import Compose dari Git.
3. Isi nilai dari `deploy/.env.example` melalui secret/config Coolify. Tidak ada fallback credential production. Perhatikan SSL mode yang sesuai resource DB; jangan mengasumsikan setiap koneksi privat memakai TLS tanpa memverifikasinya.
4. Hubungkan Compose ke predefined destination network yang sama dengan DB, lalu verifikasi private DNS/port. Baca [networking resmi Coolify](https://github.com/coollabsio/coolify-docs/blob/main/content/docs/core/networking-in-coolify.mdx).
5. Publikasikan domain HTTPS hanya untuk web dan API, masing-masing port 3000/8080. AI port 8001 dan DB tidak memiliki domain/published host port.
6. Jalankan migration **sekali sebagai release task**, sebelum trafik aplikasi. Di container API gunakan goose dengan `GOOSE_DRIVER=postgres`, `GOOSE_DBSTRING` dari secret migration role, `GOOSE_MIGRATION_DIR=/app/migrations`, lalu `goose up`.
7. Verifikasi health, origin, runtime config dan backup/restore. Health AI di Compose menggunakan liveness; readiness model masih 503 dan bukan readiness biometrik produksi.

Belum ada deployment ke akun/server Coolify pada tahap ini. Domain, credential production, legal policy, model license dan pilot mengikuti gate pada rencana induk.

## Audit dependency

CI menjalankan govulncheck dan pip-audit. npm audit menghasilkan artifact dan warning yang harus direview sebelum rilis; job tersebut tidak dinyatakan sebagai audit bersih. Temuan upstream yang belum memiliki patch kompatibel dicatat dalam [setup stack](docs/STACK-SETUP.md). Jangan menjalankan `npm audit fix --force` tanpa menilai perubahan major/downgrade yang ditawarkannya.
