# Staging: pemasangan dan verifikasi

Jalankan dari revisi rilis yang sudah lolos pemeriksaan. Gunakan data dummy untuk UAT awal. Dokumen ini tidak menyatakan bahwa deployment Coolify atau evaluasi kamera sudah dilakukan.

## Stack yang diverifikasi pada 15 Oktober 2026

| Komponen | Versi/pilihan |
|---|---|
| Node.js | 26.12.0, diverifikasi dari indeks rilis resmi dan checksum distribusi |
| Nuxt / Vue | 4.6.1 / 3.5.44, diperiksa dengan CLI upgrade resmi dan registry npm |
| shadcn-vue / Reka / Tailwind | 2.8.2 / 2.11.0 / 4.3.3; komponen yang sudah ada dipertahankan |
| TypeScript | 6.0.3; versi 7 belum menyediakan API yang dibutuhkan tooling Vue |
| Go / pgx | 1.27.2 / 5.11.0; puddle diperbarui ke 2.2.3 |
| PostgreSQL | 18.6; tidak berpindah ke PostgreSQL 19 RC |
| Python | 3.14.8; ONNX Runtime saat ini belum menyediakan wheel CPython 3.15 |
| FastAPI / Starlette | 0.143.1 / 1.8.0, dependency terkunci oleh uv |
| Playwright | 1.64.1, hanya dependency pengujian |
| uv / Goose / image API | 0.13.1 / 3.29.0 / Alpine 3.24.2 |

Sumber: [Nuxt upgrade](https://nuxt.com/docs/4.x/api/commands/upgrade), [Node 26.12.0](https://nodejs.org/en/blog/release/v26.12.0), [batas tooling Vue pada TypeScript 7](https://devblogs.microsoft.com/typescript/announcing-typescript-7-0/), [Python 3.14](https://peps.python.org/pep-0745/), [ONNX Runtime](https://pypi.org/project/onnxruntime/), [PostgreSQL](https://www.postgresql.org/support/versioning/). Versi terbaru yang kompatibel diprioritaskan; prerelease dan upgrade yang memutus tooling tidak dipasang.

## Database dan jaringan

1. Buat PostgreSQL terpisah di environment staging. Database dan layanan AI hanya berada di jaringan privat; jangan publikasikan port AI/DB. Gunakan runtime role dan migrator yang berbeda.
2. DBA memasang extension yang diperlukan migrasi. Migrator memiliki hak DDL pada schema aplikasi; runtime hanya mendapat `CONNECT`, `USAGE` schema, DML tabel, dan akses sequence yang diperlukan. Runtime tidak memerlukan superuser, `CREATEROLE`, atau hak DDL. Setelah migrasi, terapkan grants pada tabel/sequence serta default privileges milik migrator untuk migrasi berikutnya. Buat password melalui UI secret/psql `\password`, jangan menuliskannya di SQL yang di-commit.
3. Isi secret/config dari `deploy/.env.example`, termasuk dua URL database, exact `WEB_ORIGIN`, URL API publik, service token, serta kunci enkripsi wajah. Kunci enkripsi harus dibackup terpisah dari database; kehilangan kunci membuat template lama tidak dapat dibaca.
4. Hubungkan API dan migrator ke destination network database yang benar. Pada Coolify pilih Base Directory `/` dan Docker Compose Location `/deploy/compose.application.yml`. Build context memakai root checkout karena Coolify memberikan `--project-directory` ke root tersebut; perintah lokal di bawah juga wajib memakai `--project-directory .`. Periksa konfigurasi yang dirender. Domain web menuju port internal 3000 dan domain API menuju 8080. Contoh `https://attendance.example.com:3000` di konfigurasi Coolify memakai 3000 sebagai port tujuan, bukan port publik browser.
5. Gunakan HTTPS dengan web/API pada susunan site yang kompatibel dengan cookie SameSite. Verifikasi cookie `__Host-attendance_session` memiliki Secure, HttpOnly, Path=/ dan tanpa Domain. Uji WebSocket, kamera, origin ditolak, serta logout dari browser nyata.

Referensi: [Compose di Coolify](https://coolify.io/docs/applications/builds/docker-compose), [Compose run sekali jalan](https://docs.docker.com/reference/cli/docker/compose/run/).

## Instalasi pertama: migrasi → bootstrap → API

API memang berhenti bila sekolah belum dibuat. Bootstrap harus memakai container sekali jalan dan tidak bergantung pada container API yang sehat.

Contoh Compose biasa dari root repo, dengan file secret berizin 0600 di luar Git:

```bash
STAGING_ENV_FILE=/private/attendance-staging.env
docker compose --project-directory . -p attendance-staging --env-file "$STAGING_ENV_FILE" -f deploy/compose.application.yml build
docker compose --project-directory . -p attendance-staging --env-file "$STAGING_ENV_FILE" -f deploy/compose.application.yml run --rm -T migrate

read -r -s -p 'Password admin pertama: ' BOOTSTRAP_PASSWORD
printf '%s' "$BOOTSTRAP_PASSWORD" | docker compose --project-directory . -p attendance-staging --env-file "$STAGING_ENV_FILE" -f deploy/compose.application.yml run --rm --no-deps -T api bootstrap --email admin@school.example --name 'Admin Sekolah' --school-name 'Nama Sekolah'
unset BOOTSTRAP_PASSWORD

docker compose --project-directory . -p attendance-staging --env-file "$STAGING_ENV_FILE" -f deploy/compose.application.yml up -d
```

Pada Coolify gunakan **nama project, file Compose hasil render, secret, dan koneksi network yang sama dengan resource target**; jangan menciptakan project Compose kedua yang terputus dari DB. Container API boleh sedang restart karena sekolah belum ada; `run --no-deps ... api bootstrap` tetap memakai image/config API dalam container baru. Tidak perlu `docker exec` ke API yang belum sehat. Bootstrap kedua harus ditolak.

`migrate` harus selesai sukses sebelum API hidup. Jangan menggunakan `|| true` pada migrasi/bootstrap. Runner browser lokal/CI membuktikan API berhenti sebelum sekolah ada, migrasi SQL yang gagal menghasilkan kegagalan, bootstrap pertama berhasil, bootstrap ulang ditolak, kemudian API berjalan pada schema disposable. Penerapan dependency condition oleh versi Coolify target tetap harus diuji di host tersebut.

## Alamat klien dan pembatasan login

`TRUSTED_PROXY_CIDRS` menerima daftar CIDR IPv4/IPv6 yang dipisahkan koma. Isi hanya alamat/subnet proxy yang benar-benar menghubungi API, misalnya satu alamat privat dengan `/32`. Temukan melalui network/container inspect di host, jangan menyalin subnet contoh sebagai izin umum.

Kosong berarti mengabaikan forwarded headers. Wildcard `/0` ditolak. API membaca X-Forwarded-For dari kanan melalui hop yang dipercaya lalu berhenti pada klien yang tidak dipercaya; alamat kiriman klien langsung tidak mengganti identitas pembatasan login. Jika ada CDN/proxy berantai, sanitasi forwarded headers dan atur trusted IP pada proxy juga. Referensi [Traefik entrypoints](https://doc.traefik.io/traefik/reference/install-configuration/entrypoints/).

Uji dua klien berbeda melalui proxy staging: keduanya harus memiliki kuota terpisah, dan pemalsuan header dari koneksi tidak dipercaya tidak boleh melewati kuota. Pembatasan per akun tetap aktif.

## Menggunakan absensi kamera Fantech

1. Di Siswa, pastikan siswa aktif dan ditempatkan di kelas/tahun ajaran yang aktif. Di Wajah siswa, pilih siswa dan ambil 3–5 foto kamera yang berbeda hingga pendaftaran tersimpan.
2. Buat jadwal sesuai jam sekolah dan buka sesi pada tanggal yang benar. Jam sekarang harus berada di antara mulai dan tutup sesi. Sesi yang sudah ditutup tidak dibuka ulang atau diubah riwayatnya; buat sesi/jadwal yang sesuai untuk absensi berikutnya. Pastikan nama siswa ada pada daftar siswa sesi.
3. Di Perangkat absensi, gunakan perangkat aktif dan buat kode penghubung. Buka `/absensi`, pilih sesi terbuka, lalu masukkan kode sekali pakai. Pairing mengakhiri sesi akun operator pada browser itu agar kiosk tidak memiliki akses panel. Pemasangan tersimpan selama 30 hari sejak penggunaan terakhir dan mengikuti sesi aktif kelas yang sama; tanpa sesi, perangkat menunggu tanpa meminta kode baru. Lihat [alur koneksi perangkat](DEVICE-CONNECTION.md). Terapkan migrasi `00016_persistent_devices.sql` sebelum API baru; koneksi lama perlu dipasangkan ulang satu kali.
4. Pilih Fantech Luminous C30 pada pilihan Kamera, lalu Aktifkan kamera dan izinkan kamera pada browser. Pilihan kamera dipakai bersama oleh pendaftaran dan absensi. Hentikan kamera sebelum memilih perangkat lain. Audio tidak diminta.
5. Hadapkan satu wajah lurus ke lensa dengan cahaya cukup dan tahan posisi beberapa frame. Nama hanya muncul setelah server menyimpan/memastikan kehadiran. Pemindaian ulang mempertahankan satu catatan dan waktu hadir pertama; terlambat mengikuti batas sesi.
6. Periksa catatan di detail sesi dan Laporan kehadiran. Saat sesi/perangkat tidak aktif, orang tidak dikenal, gambar tidak layak atau pemeriksaan wajah hidup gagal, tidak ada kehadiran yang ditulis. Gunakan pencatatan manual bila diperlukan.

Pengujian lokal 15 Oktober 2026 berhasil menggunakan Fantech nyata hingga satu catatan hadir tersimpan; pengenalan ulang tidak menambah baris. Pendaftaran ulang dan sesi uji dilakukan atas persetujuan pemilik. Sesi uji sudah ditutup dan jadwal semula dikembalikan. Ini uji fungsional satu siswa, bukan evaluasi akurasi populasi. Uji lanjutan juga dijalankan sepenuhnya lewat browser `/absensi`: kode penghubung dibuat dari panel, sesi dipilih di browser, kamera Fantech diaktifkan, nama siswa terdaftar muncul pada kotak hijau bersama pesan kehadiran sudah tercatat, lalu baris Hadir diverifikasi pada detail sesi setelah login ulang. Tepat satu catatan tersimpan; sesi uji ditutup dan jadwal semula dipulihkan.

Webcam tersedia tidak otomatis berarti ada sesi yang sedang terbuka. Daftar sesi kosong berarti petugas perlu menyiapkan sesi pada jam yang sesuai. Pengujian kamera tunggal tidak menjadi klaim akurasi seluruh siswa atau jaminan menolak semua jenis foto/video.

## Kamera, model, dan evaluasi

Installer `python -m app.models --directory /models` memasang model dari manifest dengan ukuran dan SHA-256 tetap, beserta lisensinya. Volume model persisten dan dibaca read-only oleh AI. Penambahan model PAD juga terpasang melalui initializer yang sama; tidak ada PyTorch/GPU tambahan.

- Pendaftaran memeriksa kualitas/konsistensi 3–5 frame dan duplikat terhadap template siswa lain dalam sekolah, termasuk di luar kelas operator. Identitas siswa pembanding tidak dikirim ke browser. Template tidak diganti jika pemeriksaan gagal.
- Pengenalan memakai engine 1:N, margin, pemeriksaan keaslian wajah dan konfirmasi beberapa frame. Cache terpisah per sekolah/sesi/perangkat, dibatasi ukurannya, dan dibersihkan ketika isi gallery berubah. Satu orang per frame; beberapa wajah ditolak.
- Default threshold/margin/PAD/duplikat adalah nilai **sementara**, bukan klaim akurasi. Dataset uji harus terpisah dari pendaftaran dan kalibrasi. Tool menolak byte gambar yang berulang atau tumpang tindih dan path keluar direktori dataset; pemisahan orang, kamera, waktu, dan kondisi tetap diperiksa operator.
- Export PAD pihak ketiga mempunyai perbedaan label pada model card: source upstream memakai kelas 1 sebagai live, sedangkan card export menyebut kelas 0. Adapter mengikuti source upstream: kelas 1 live dan BGR float32 0–255, tanpa pembagian 255. Konformitas dan akurasi tetap perlu diuji pada kamera nyata. Jangan menyalin angka akurasi repository sebagai akurasi sekolah. [Source upstream](https://github.com/minivision-ai/Silent-Face-Anti-Spoofing/blob/master/test.py), [provenance export](https://huggingface.co/garciafido/minifasnet-v2-anti-spoofing-onnx).

Format dataset evaluator yang sudah ada, simpan secara privat:

```json
{
  "split": "holdout",
  "model_sha256": "SHA256_SFACE_DARI_MANIFEST",
  "gallery": {"anon-001": ["enrollment/one.jpg", "enrollment/two.jpg", "enrollment/three.jpg"]},
  "probes": [
    {"expected": "anon-001", "frames": ["known/one.jpg", "known/two.jpg", "known/three.jpg"]},
    {"expected": null, "frames": ["unknown/one.jpg", "unknown/two.jpg", "unknown/three.jpg"]},
    {"expected": "anon-001", "attack": true, "attack_type": "print", "frames": ["print/one.jpg", "print/two.jpg", "print/three.jpg"]},
    {"expected": "anon-001", "attack": true, "attack_type": "replay", "frames": ["replay/one.jpg", "replay/two.jpg", "replay/three.jpg"]}
  ]
}
```

Setiap path mengacu pada JPEG berbeda relatif terhadap file dataset. Gunakan ID anonim, bukan nama siswa. Contoh ini hanya format, bukan jumlah sampel yang memadai.

```bash
cd services/ai
../../.tools/uv run python -m app.engine_tools calibrate --models ../../.tools/models --dataset /private/calibration/data.json --authorized-dataset --threshold 0.8 --margin 0.1 --with-pad
../../.tools/uv run python -m app.engine_tools evaluate --models ../../.tools/models --dataset /private/holdout/data.json --authorized-dataset --threshold 0.8 --margin 0.1 --with-pad --liveness-threshold 0.95 > /private/holdout-report.json
```

Evaluator menghasilkan metrik identifikasi, orang tak dikenal, serangan, latency offline, CPU, dan memori. Sampling delay dilewati pada evaluasi offline; ukur juga latency **kamera-ke-UI** pada perangkat sebenarnya. Hasil kalibrasi tidak boleh dipakai sebagai hold-out. Benchmark sintetis `app.engine_tools benchmark` hanya mengukur tahap buatan, bukan akurasi atau kapasitas dua kiosk.

### Laporan evaluasi terpisah dari operasi

`AI_RECOGNITION_APPROVAL_PATH` dan `AI_RECOGNITION_APPROVAL_SHA256` bersifat opsional untuk melampirkan laporan evaluasi yang sudah ditinjau. Ketiadaan laporan tidak lagi mematikan pengenalan. Keputusan operasional tetap wajib satu wajah, pencocokan stabil, PAD lulus, model cocok, perangkat sah dan siswa anggota sesi aktif. Status akurasi tetap belum dievaluasi sampai tersedia bukti independen.

Setelah evaluasi hold-out ditinjau, laporan harus cocok dengan SHA detector/embedding/PAD, fingerprint kode engine, dan seluruh `EngineConfig` runtime. Hanya satu hasil hold-out diterima, bukan sweep kalibrasi. Penandaan laporan sebagai ditinjau memerlukan pengujian known/unknown/serangan, nol salah identifikasi/false accept/serangan lolos, kategori print dan replay, serta batas atas 95% untuk nol false accept ≤0,1%. Itu memerlukan setidaknya sekitar 2.995 probe unknown independen; jumlahnya tidak boleh dipenuhi dengan menyalin gambar. FNIR dan sebaran sampel juga harus ditinjau.

Tambahkan `review` pada salinan laporan **hanya setelah** pemeriksaan manusia dan pengukuran perangkat:

```json
{
  "automatic_attendance_approved": true,
  "holdout_verified": true,
  "liveness_verified": true,
  "stability_verified": true,
  "threshold_margin_verified": true,
  "liveness_threshold": 0.95,
  "camera_to_ui_p95_ms": 1400
}
```

Angka 1400 di atas hanya contoh format; ganti dengan hasil pengukuran, jangan menyalinnya sebagai bukti. Hitung `sha256sum /private/reviewed-report.json`, mount file tersebut read-only untuk AI, lalu isi dua konfigurasi approval dengan path dan hash tepatnya. Compose dapat memakai file pada volume `/models` yang dikelola operator. Restart AI setelah perubahan approval/config. Ambang PAD yang dicatat evaluator harus sama dengan konfigurasi runtime dan nilai pada review; threshold/margin hasil uji juga harus sama dengan konfigurasi engine. Laporan hilang, rusak, kurang sampel atau tidak cocok tidak ditandai sebagai sudah ditinjau. Go tetap memeriksa bukti runtime wajah hidup/konfirmasi, identitas/model siswa, sesi, perangkat, dan idempotensi sebelum transaksi.

## Dependency dan CI

`npm audit` masih melaporkan tiga advisory asal pada tool build upstream yang belum memiliki versi perbaikan. Pengecualian di `deploy/dependency-exceptions.json` **berakhir 15 November 2026**. CI menyimpan hasil audit penuh dan gagal untuk advisory baru, pengecualian kedaluwarsa, atau paket terdampak yang ditemukan dalam metadata output runtime. Ini pemeriksaan artefak/metadata, bukan bukti formal bahwa semua kode inline bebas kerentanan. Input build dibatasi pada kode/config tepercaya; jangan menerima pola glob, sertifikat development, atau source map dari pengguna.

Jangan downgrade Nuxt atau menjalankan `npm audit fix --force` hanya untuk menghilangkan angka audit. Perbarui paket upstream ketika patch tersedia dan hapus pengecualian. Scan Python/Go juga tetap dijalankan.

## Pemeriksaan lokal yang dapat diulang

```bash
make check
make build
make compose-check
# DATABASE_URL harus menunjuk database lokal/CI yang diizinkan; runner membuat schema tersendiri.
E2E_DATABASE_URL="$DATABASE_URL" npm --prefix apps/web run test:e2e
```

Playwright memerlukan `npx playwright install --with-deps chromium --only-shell` pada mesin baru, dijalankan dari `apps/web`. Runner browser memakai port 3007/8087 dan schema acak, bukan data aplikasi utama. Jangan menjalankannya bersamaan pada port yang sama. Suite membuktikan login inline, foto lintas origin, siswa dengan akses kelas, laporan khusus peran, dan keputusan koreksi pengguna lain.

## Operasional sebelum membuka staging

- Backup terenkripsi database serta kunci enkripsi ke lokasi terpisah; jadwalkan backup melalui fasilitas host/Coolify dan uji restore pada database staging terpisah. Backup lokal yang sudah pernah diuji tidak menggantikan uji restore host target. Catat RPO/RTO dan siapa yang memulihkan layanan.
- Simpan volume model/checksum/lisensi, atau buktikan installer dapat mengisi volume kosong. Jangan mengganti nama project Compose tanpa rencana pemindahan volume.
- Pantau `/api/v1/health/live`, `/api/v1/health/ready`, health/readiness AI, restart, error, kapasitas disk dan umur backup. Log aplikasi tidak boleh menyimpan frame, embedding, token atau password.
- Uji 500 siswa dan dua kiosk pada host target, putus jaringan, sesi habis, akses dicabut, hari libur dan kondisi kamera. Satu API replica tetap menjadi batas desain; jangan menambah replica tanpa menilai limiter dan lifecycle worker.
- Gunakan image/revisi yang sama saat tes, deployment dan rollback. Rollback aplikasi tidak otomatis membalik migrasi; utamakan migrasi kompatibel dan restore yang sudah diuji bila diperlukan.

Aktivasi kamera, akurasi nyata, uji beban, TLS/proxy dan restore host Coolify masih merupakan pemeriksaan lingkungan, bukan hasil yang dapat dinyatakan dari tes sintetis lokal.


## Perbaikan audit keamanan dan UI — 15 Oktober 2026

Migrasi `00013_capture_freshness.sql` wajib diterapkan sebelum API/web baru dipakai. Runtime role perlu DML pada `kiosk_capture_challenges`; tabel ini tidak menyimpan foto, hanya izin pengambilan dan fingerprint. Worker menghapus data yang berakhir lebih dari 24 jam lalu. Perangkat lama dipasangkan ulang bila izinnya dicabut; perubahan role/email/status akun, password sensitif, dan pencabutan penugasan mencabut credential perangkat terkait. Izin operator serta cakupan kelas diperiksa pada setiap pemakaian dan setelah inferensi. Masa berlaku credential operator dengan cakupan kelas dibatasi akhir penugasan saat pemasangan; pemberian penugasan baru tidak mengaktifkan credential lama.

Klien kamera meminta POST `/api/v1/kiosk/capture-challenge`, mengambil gambar baru, kemudian memakai `event_id` balasan untuk recognize. Izin terikat sesi perangkat, berlaku sepuluh detik, dan sekali pakai. `CAPTURE_REPLAY`/`RECOGNITION_STALE` berarti ambil gambar baru; jangan mengirim ulang foto lama. Perubahan galeri selama inferensi membatalkan hasil. Inferensi berlangsung di luar transaksi yang mengunci sekolah.

Proteksi ini menolak replay gambar identik dan metadata lama, tetapi browser/webcam biasa tidak memberikan bukti hardware bahwa foto berasal dari siswa yang sedang hadir. Foto yang dimodifikasi, rekaman video, kamera virtual, serta spoof fisik tetap memerlukan evaluasi independen dan pengawasan petugas. Threshold model tidak diubah oleh perbaikan keamanan ini.

Web menetapkan CSP bernonce, larangan framing, nosniff, same-origin referrer, Permissions-Policy kamera milik origin sendiri, dan HSTS tanpa includeSubDomains. Zod memakai jitless untuk bekerja tanpa unsafe-eval. Style inline tetap diizinkan karena chart dan primitive menggunakannya. `NUXT_PUBLIC_API_BASE` menentukan origin API/WebSocket yang diizinkan; jangan gunakan wildcard. Saat staging, verifikasi header respons HTTPS aktual, jalur login, tema, laporan, WebSocket dan kamera; proxy tidak boleh menghapus atau menggandakan CSP yang bertentangan.

Untuk beberapa replica API, pembatasan login per proses belum menjadi kuota bersama: pasang pembatasan di reverse proxy/shared limiter sebelum menaikkan replica. Uji dua klien dan header IP proxy. Tetap gunakan satu API replica sampai kontrol itu tersedia. Image OS scan, grant runtime DB, TLS, backup/restore dan UAT kamera dilakukan pada target staging menggunakan data dummy; hasil lokal tidak menggantikan pemeriksaan tersebut.

## Kalender libur Indonesia — 16 Oktober 2026

Terapkan migrasi 14 (`school_holidays`, `school_holiday_sync`) sebelum API baru. Runtime role memerlukan DML pada kedua tabel. Sumber tetap `https://indonesian-holidays.ahrulsyamil.com/holidays/{year}`; backend mengirim tahun saja, tanpa identitas sekolah/siswa. Sediakan outbound HTTPS ke sumber. UI dan absensi membaca cache database, bukan meminta sumber pada setiap pemindaian.

Worker memperbarui tahun sekolah berjalan dan tahun berikutnya setiap 24 jam; tahun gagal/kosong dicoba kembali paling cepat setelah 1 jam. Tombol **Perbarui libur nasional** memperbarui tahun bulan yang sedang dilihat. Data harus valid dan tidak kosong; respons kosong (misalnya 2027 pada saat verifikasi) tidak membuktikan tahun bebas libur. Cache lama tetap berlaku. Tahun tanpa cache menahan pembuatan/pembukaan sesi otomatis; pengaturan sekolah eksplisit tetap dapat dipakai. Periksa peringatan kalender ketika mengganti tahun ajaran.

Prioritas hari: pengaturan sekolah → libur nasional/cuti bersama → weekday jadwal kelas. Dialog **Tambah libur sekolah** mendukung rentang inklusif maksimal 366 hari untuk libur semester dan hari belajar pengganti. **Gunakan kalender otomatis** menghapus override satu tanggal. Perubahan tidak membuka/menutup ulang sesi yang sudah berjalan atau menulis ulang kehadiran. Sesi terjadwal yang menjadi tidak efektif tidak dibuka; setelah batas tutup dibatalkan tanpa menghasilkan alfa.

Pengelolaan kalender berlaku seluruh sekolah dan memerlukan `academics.write` dengan scope `school`. Cache diperbarui setelah pemeriksaan ulang izin petugas, dan audit tersimpan dalam transaksi yang sama. Provider lambat tidak menahan kunci sekolah. Sinkronisasi manual tidak tersedia bersamaan dengan sinkronisasi lain pada instance API yang sama; batas satu replica pada runbook tetap berlaku.
