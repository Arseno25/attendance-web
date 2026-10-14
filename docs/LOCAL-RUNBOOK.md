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

Pendaftaran wajah menggunakan Wajah siswa (`/admin/biometrics`) untuk petugas berizin. Siswa aktif dapat didaftarkan tanpa persetujuan wali di lokal maupun produksi. Ambil 3–5 foto berbeda yang lolos pemeriksaan kualitas. Penghapusan manual atau penonaktifan siswa menghapus data wajah; riwayat persetujuan lama tidak lagi menentukan kelayakan. G2/G3 tetap memerlukan evaluasi sebelum absensi otomatis.

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


## Reset data demo lokal — 14 Oktober 2026

Atas permintaan pengguna, fixture demo akademik/absensi dihapus dari database preview lokal: 1 siswa DEMO-001, 1 kelas DEMO-7A, 1 tahun ajaran, 1 jadwal, 3 sesi, 1 penempatan, 1 catatan/event absensi, 1 ledger request dan 1 kalender demo. Identitas dibatasi UUID/name/code yang telah diperiksa; tidak menggunakan TRUNCATE atau menonaktifkan seluruh constraint. Tidak ada wali, consent, tombstone atau template biometrik untuk fixture ini. Identitas tenant, akun/password/sesi/grants pengguna dan perangkat `tes` dipertahankan. Audit historis tetap append-only dan mendapat event `development.seed_reset` baru.

Sebelum perubahan, dump PostgreSQL18 format custom dialirkan langsung ke GnuPG AES256; backup terenkripsi `.tools/sidebar-reset/pre-reset.dump.gpg` dan kunci pemulihan terpisah di directory private ignored dengan permission 0600. Dekripsi dan daftar TOC `pg_restore --list` diverifikasi. Backup tidak dibuka ke browser atau Git.

Reset diuji dahulu dalam transaksi ROLLBACK. Identitas fixture, dependensi tidak terduga, perbandingan seluruh baris akun/perangkat/sekolah/audit dan trigger enabled diperiksa sebelum COMMIT aktual. Empat trigger riwayat pada tabel fixture saja ditangguhkan dalam transaksi dan diaktifkan kembali sebelum commit; FK dan proteksi audit tetap aktif. Setelah reset, semua tabel demo operasional tersebut kosong, akun dan perangkat masing-masing tetap 1. Log/rencana SQL lokal di `.tools/sidebar-reset/` (ignored), tidak menjadi endpoint reset atau migrasi produk. File seed pengembangan hanya memastikan tenant dummy dan tidak membuat ulang fixture akademik/absensi.


## Pendaftaran wajah — 14 Oktober 2026

Pengelolaan kamera dipusatkan di **Wajah siswa** (`/admin/biometrics`). Cari nama/NIS, klik ikon wajah, aktifkan kamera, ambil 3–5 foto berbeda, lalu simpan. Modal menampilkan status dan kamera. Detail siswa tidak memuat bagian wali atau persetujuan. API guardian/consent/withdraw dan permission `consents.manage` telah dihapus.

Kebijakan berlaku sama di lokal dan produksi tanpa flag khusus. Template baru memakai konteks enkripsi null. Migrasi 00011 mempertahankan ciphertext, ID konteks enkripsi template lama, dan riwayat wali/persetujuan. Expiry/withdrawal lama tidak lagi menghapus atau memblokir template. Siswa nonaktif dan penghapusan manual tetap menutup pendaftaran dan galeri. Rollback kebijakan memerlukan backup terverifikasi.

Akurasi dan pemeriksaan keaslian wajah belum dievaluasi; absensi otomatis tetap tertutup. Kamera dan data wajah nyata tidak dipakai untuk tes perubahan ini.

Verifikasi: tes API dengan PostgreSQL nyata dan `-race` lolos, termasuk expiry/withdrawal historis yang tidak lagi memblokir galeri serta penonaktifan siswa yang tetap menghapus template. Tes AI menerima konteks baru/null dan konteks enkripsi lama. Browser memverifikasi status wajah lama tetap terbaca, tidak ada bagian wali di detail siswa, tema tersimpan setelah reload, serta lebar halaman ponsel tanpa overflow. Backup terenkripsi terverifikasi dan fingerprint data sebelum/sesudah migrasi identik.

Tema berada di **Pengaturan → Tampilan**: Terang, Gelap, Sistem; pilihan otomatis tersimpan per peramban. Semua pengguna login dapat mengakses Tampilan. Urutan tab adalah Sekolah lalu Tampilan; Sekolah terbuka pertama bagi pengelola berizin, sedangkan pengguna lainnya langsung membuka Tampilan. Tab Sekolah dan API pengaturan sekolah tetap memerlukan pengelola penuh dengan izin `school.write`.

## Identitas sekolah dan foto profil — 14 Oktober 2026

Pengelola penuh dengan izin `school.write` membuka Pengaturan sekolah untuk mengubah nama sekolah, nama aplikasi, alamat, telepon, email, dan logo. Nama dan logo tampil di halaman masuk dan sidebar; alamat/kontak hanya tersedia melalui endpoint pengaturan yang berizin. Waktu sekolah tidak diubah. Akun saya menyediakan foto profil pengguna sendiri, pratinjau gambar, simpan, dan hapus; foto tampil di menu akun tanpa mengubah password atau izin.

Endpoint baru: `GET /api/v1/school/branding` dan `GET /api/v1/school/logo` untuk identitas publik; `GET/PATCH /api/v1/school/settings` dan `PUT/DELETE /api/v1/school/logo` untuk pengelola; `GET/PUT/DELETE /api/v1/auth/profile/photo` untuk pemilik akun. Semua penulisan menggunakan origin/CSRF dan pemeriksaan sesi/izin terkini. Foto akun memakai `private, no-store` dan tidak dapat dibaca akun lain.

JPG/PNG maksimal 2 MB dan 12 megapiksel diperiksa sebelum decode, orientasi JPEG EXIF/PNG eXIf diterapkan, gambar diperkecil hingga 512 piksel, kemudian di-encode ulang tanpa metadata. Binary tersimpan di PostgreSQL dan ikut backup database. Audit hanya mencatat tindakan, bukan file atau kontak pribadi. Migrasi 00010 menambahkan kolom dengan default dan tidak mengubah akun maupun data wajah sebelumnya. Rollback ditolak bila pengaturan/gambar baru masih berisi data.

Teks antarmuka menggunakan peran, hak akses, daftar siswa, kode penghubung, dan foto pendaftaran. Detail implementasi tetap berada di API/log untuk pengembang. Tidak ada pengunggahan foto atau perubahan identitas sekolah pengguna selama verifikasi browser; pengujian upload menggunakan gambar sintetis dalam schema database sementara.

Verifikasi akhir: seluruh tes Go dengan `-race`/database nyata, 51 tes UI Node, 65 tes AI, dan satu tes Python lolos; lint/typecheck/build/Go vet juga lolos. Delapan orientasi JPEG, orientasi PNG, metadata rusak, batas gambar, akses foto antar-akun, dan CSRF diuji. Browser desktop/mobile memperlihatkan form pengaturan/profil tanpa overflow, pemilih gambar berbahasa Indonesia, dan identitas sekolah di halaman masuk; tidak ada error console. Backup terenkripsi berhasil diverifikasi sebelum migrasi 00010; fingerprint data sekolah, akun, dan wajah lama tetap sama. Tiga fixture form lama yang sebelumnya gagal sudah disesuaikan dengan schema peran saat ini.
