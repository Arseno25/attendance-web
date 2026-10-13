# Operasi lokal dan langkah menuju rilis

Preview workspace: web `http://localhost:3001`, API loopback 8080, AI loopback 8001, PostgreSQL loopback 54329. Data pengembangan dummy; runtime bukan deployment produksi. Jangan menjalankan `down -v`, `git clean -fdx` atau restore di atas database aktif. Fokus lokal mengikuti arahan pengguna.

## Menjalankan dan memeriksa

`make setup`, `make db-up`, `make migrate`, `make check`, `make db-test`, `make build`, `make compose-check`. Development: `make dev-api`, `make dev-web`, `make dev-ai`. Hentikan hanya preview container project ini bila port yang sama akan digunakan development. Health ready API hanya membuktikan koneksi DB; AI ready200 menunjukkan model/kunci enrollment siap; recognition tetapfalse sampai G2/G3.

Pasang baseline model tanpa upload wajah:

```bash
cd services/ai
../../.tools/uv run python -m app.models --directory ../../.tools/models
```

Manifest di `services/ai/models/manifest.json` memakai commit/ukuran/checksum dari OpenCV Zoo, model YuNetMIT/SFaceApache2.0; installer menyimpan LICENSE di directory model. Tidak mengubah model menjadi production-ready. Artefak model ignored, tidak ikut Git. Raw frame dan embedding plaintext tidak disimpan; enrollment sekarang menyimpan ciphertext AES-GCM. Pertahankan kunci di secret terpisah dari backup DB.

Evaluasi trace anonim dari hold-out yang diizinkan:

```bash
cd services/ai
../../.tools/uv run python -m app.evaluation /absolute/private/authorized-holdout.json
```

Input array `{expected: anonymous_subject_id|null, predicted: anonymous_subject_id|null, latency_ms: finite_number, attack: boolean}`. Tidak menggabungkan attack dengan unknown probes FPIR. Laporan berisi known/unknown/attack counts, FPIR/FNIR, wrong-known IDs count (tanpa identitas), attack pass rate, P95 nearest-rank, dan upper95 untuk zero false accepts. Sampel sedikit tidak membuktikan sasaran0.1%; report tidak pernah memberi persetujuan automatic attendance. Camera hardware, quality/liveness/stability dan actual end-to-end latency masih harus diuji sebelum G2.

## Alur operator

1. Admin mendaftarkan device di `/admin/devices`, lalu menerbitkan token pairing5menit sekali pakai.
2. Di perangkat tujuan operator login, membuka `/absensi`, memilih sesi terbuka dan memasukkan token. Cookie device10menit; setelah pairing sesi akun operator dicabut dalam transaksi yang sama dan cookie akun dihapus. Perangkat publik hanya memegang credential device. Login ulang diperlukan untuk re-pairing.
3. Aktifkan kamera untuk deteksi frame dan kotak wajah. Kamera berhenti pada hidden/offline/expiry/leave; izin kamera ditolak menampilkan pesan. Kotak unknown tidak membocorkan nama kandidat; kehadiran otomatis belum aktif.
4. Revoke pada panel langsung menghapus pairing/session; heartbeat berikutnya menghentikan kiosk. Jika layanan/kamera gagal, petugas menggunakan manual attendance. Jika API/DB down, pakai SOP administratif offline, kemudian input setelah pulih.

Wali/consent di detail siswa hanya untuk officer dengan `consents.manage`. Verifikasi dilakukan lewat prosedur sekolah; record menyimpan referensi privat, tidak mengunggah scan identitas. Withdrawal langsung menutup eligibility. Petugas enroll/revoke memakai `/admin/biometrics` tanpa akses akademik tambahan. Ambil3–5frame berbeda untuk enrollment provisional; status wajah harus menunjukkan consent berlaku. Ciphertext dihapus saat withdrawal/revoke/siswa nonaktif, dan worker menghapus ciphertext consent expired. Gallery eligible dibaca dari DB per frame, tanpa cache template. G2/G3 tetap memerlukan evaluasi/restore produksi.

Import di `/admin/students/import`: satu kelas, UTF8CSV header `nis,nisn,full_name`, max500rows/120KiBfile. Preview15menit, creator-only commit atomik; siswa duplikat/kelas invalid menyebabkan rollback. Payload expired dipurge worker setiap1menit; committed segera dihapus. Retry committed sama tidak membuat duplikat.

Pulang di detail sesi: staff verifikasi, alasan wajib, hadir/terlambat saja; clockserver dan first timestamp dipertahankan. Approved correction menghapus checkout summary lama (event tetap historis). Live update per-sesi maksimal lag2detik, hanya invalidation; RESTrefetch saat reconnect/visible. Tetap gunakan reload bila WebSocket tidak tersedia.

## Backup, restore dan rollback

Backup harus memakai `pg_dump -Fc` dengan client major18 yang sama dengan database, dienkripsi sebelum ditulis ke disk. Gunakan key/passphrase melalui file private0600 atau managed secret/stdin, bukan argumen/log. Directory backup0700; verifikasi ciphertext integrity sebelum memberikan plaintext ke `pg_restore`. Jangan menyimpan key bersama backup produksi.

Rehearsal lokal13Oktober2026 memakai GnuPG AES256symmetric, dump di pipe (tanpa file plaintext), decrypt terverifikasi lalu restore ke database disposable bernama acak. Membandingkan schema version6 dan counts students/attendance/tombstone; drop hanya database rehearsal; key dan artifact latihan dihapus. Bukti `.tools/local-restore-evidence.json`. Ini tidak membuktikan backup schedule/RPO/RTO/remote storage/key recovery produksi.

Restore produksi nanti wajib mematikan recognition, replay semua tombstones dari sumber revocation yang lebih baru daripada backup, purge revoked ciphertext, rebuild gallery eligible dan validasi revisionACK sebelum mengaktifkan inference. Latihan sebelumnya hanya menguji core relational restore schema6. Runtime sekarang menyimpan ciphertext schema7; rehearsal tersebut belum membuktikan purge template setelah restore. Jangan menyatakan G3 lulus dari latihan ini.

Rollback aplikasi: pilih binary/image kompatibel schema7. Migration5/6/7 menambah tabel, constraint, endpoint; rollback ke core sebelum checkout harus dinilai setelah record checkout dibuat. Tidak menjalankan downmigration pada data operasional. Koreksi forward lebih aman; jangan memakai restore DB rutin karena dapat menghidupkan consent/data lama.

## Insiden dan rilis

- API/DB outage: no automatic success; manual offline administratif, reconcile setelah pulih.
- Device compromise: revoke pada panel, pastikan heartbeat/session401, re-pair device baru setelah investigasi.
- AI/model unavailable: manual terus berjalan; feature tetapdisabled; jangan turunkan threshold untuk demo.
- Salah match/purge backlog/key compromise setelah biometrik aktif: disable recognition, simpan audit nonbiometrik, ikuti prosedur sekolah; investigasi/rekey/purge/rebuild sebelum membuka ulang.

Coolify deployment, HTTPS cookies/CORS pada domain sebenarnya, RBAC least-privilege DB roles, encrypted backup schedule/RPO/RTO, load test500gallery/2kiosk, UAT/training/DPIA/pilotsign-off tetap gate produksi. Tidak dilakukan dalam pekerjaan lokal ini. WP12 adalah kegiatan sekolah nyata, bukan fungsi yang dapat ditandai selesai dari fixture.

## Akun, tema dan lingkungan pengembangan

Menu akun membuka `/admin/account`. Nama dapat diperbarui tanpa password; perubahan email/password memerlukan password saat ini dan mencabut semua sesi lama. Pilihan tema terang/gelap/sistem memakai color-mode dan shadcn DropdownMenu, disimpan per browser.

Kunci `AI_TEMPLATE_KEY_BASE64` adalah32byte acak base64, `AI_TEMPLATE_KEY_ID` identitas kunci. Kunci hanya diteruskan ke AI; API memakai `AI_BASE_URL` privat dan token internal yang sama. Model dimount read-only. Jangan mengganti kunci tanpa rencana pendaftaran ulang. `make setup` tidak menimpa `.env` existing; lengkapi variabel baru secara privat.

Karena disk home hampir penuh saat verifikasi14Oktober2026, virtualenv project dipindahkan ke `/tmp/attendance-ai-venv` dan disymlink; virtualenv development bersifat sementara. Pulihkan setelah reboot dengan `UV_PROJECT_ENVIRONMENT=/tmp/attendance-ai-venv .tools/uv sync --project services/ai --locked`. Ini tidak memindahkan DB, model atau secret. Preview AI sekarang menggunakan image yang dibangun dari Dockerfile/lockfile, tanpa mount virtualenv host.

Import image ke storage home kehabisan ruang. Image AI dibangun pada storage tambahan `/var/tmp/attendance-ai-build/storage` (disk root) dan digunakan read-only. Pertahankan directory ini selama preview memakai image tersebut; jangan masukkan ke pembersihan temp. Untuk menjalankan kembali preview AI:

```bash
podman --storage-opt additionalimagestore=/var/tmp/attendance-ai-build/storage start attendance-foundation-ai
```

Build/Compose di host yang cukup ruang dapat memakai storage standar. Root filesystem dan home sama-sama terbatas; ini hanya pengaturan preview workspace lokal.
