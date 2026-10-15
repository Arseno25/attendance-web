# Audit kesiapan staging — 15 Oktober 2026

**What this repo does:** Aplikasi ini mengelola siswa, kelas, jadwal, pencatatan kehadiran, koreksi, laporan, dan perangkat absensi sekolah. Panel memakai Nuxt/Vue, API Go, PostgreSQL, dan layanan Python untuk kamera serta pendaftaran wajah. Acuan beban audit: satu sekolah, sekitar 500 siswa, 1–2 kiosk, dan satu replika API.

**Keputusan pada baseline audit:** belum siap untuk uji pengguna di staging sebelum temuan wajib di bawah diperbaiki. Staging administrasi dengan data dummy dapat menjadi tahap pertama setelah perbaikan itu. Absensi wajah otomatis memiliki pekerjaan implementasi dan evaluasi tersendiri; jangan dinyatakan siap hanya karena kamera dan pendaftaran wajah sudah berjalan.

## Pembaruan setelah perbaikan — 15 Oktober 2026

Bagian temuan di bawah merupakan rekaman baseline sebelum perbaikan. Status kode sekarang:

- Temuan 1–7 ditutup dengan perbaikan server/UI, regresi database terisolasi, dan urutan migrasi → bootstrap → API yang diuji dari kosong. Lingkup kelas, larangan menyetujui sendiri, histori absensi dan validasi tetap dipertahankan.
- Temuan 8: seluruh perubahan disiapkan pada branch lokal `codex/staging-readiness`; gunakan commit rilis final, bukan SHA baseline. Push, CI remote pada revisi baru, dan deployment Coolify belum dijalankan.
- Temuan 9: dependency kompatibel diperbarui. Tiga advisory upstream tool build belum memiliki patch; pengecualian eksplisit berakhir 15 November 2026. CI kini menolak advisory baru, pengecualian kedaluwarsa, atau paket terdampak dalam metadata output runtime. Pemeriksaan metadata tidak membuktikan ketiadaan seluruh kode yang mungkin dibundel inline.
- Temuan 10: tersedia tes browser Chromium nyata untuk login/foto lintas origin, peran siswa minimum, ekspor laporan terbatas kelas, keputusan koreksi dan pencatatan hadir–pulang hingga laporan. Proxy HTTPS, kamera fisik dan WebSocket pada host tujuan tetap perlu smoke test.
- Engine wajah sekarang terhubung ke endpoint pengenalan, galeri terenkripsi, tracking, pemeriksaan pendaftaran ganda dan model pemeriksaan keaslian wajah. Identitas/kehadiran otomatis tetap ditahan tanpa laporan hold-out yang ditinjau, hash tepat, model/config/code fingerprint cocok, serta bukti latensi dan penolakan foto/video. Ambang PAD hasil evaluator harus sama dengan konfigurasi runtime dan review.
- Koreksi cakupan audit: runner dataset `app/engine_tools.py` sudah ada pada baseline. Perbaikan memakai dan memperluas runner tersebut; ringkasan `evaluation.py` bukan satu-satunya alat evaluasi. Tidak ada dataset kamera terpisah yang diberikan dan tidak ada angka akurasi nyata yang diklaim.
- Struktur service Go, composable Vue dan class Python yang sudah ada dipertahankan; tidak ada lapisan/pola tambahan yang dipaksakan. CLI, konfigurasi dan versi stabil kompatibel dijelaskan pada [runbook staging](STAGING-RUNBOOK.md).

**Penggunaan absensi:** pencatatan hadir/izin/sakit manual, pulang, koreksi, sesi dan laporan dapat diuji lokal. Pendaftaran wajah melalui kamera tersedia; pencatatan wajah otomatis membutuhkan evaluasi perangkat terlebih dahulu. Pengaturan bawaan sementara tidak dianggap sebagai bukti akurasi.

**Verifikasi revisi perbaikan:** `make check` lulus (Go race dan integrasi database, 80 pytest, 58 tes Node, 5 unittest skrip), serta lima tes browser Chromium; `make build` dan validasi Compose lulus. Python tanpa kerentanan diketahui; Go tanpa pemanggilan rentan. Pemindaian source yang akan di-commit tidak menemukan secret. Runtime lokal memakai overlay source/dependency di atas image lokal yang sudah ada; build Docker bersih pada CI revisi baru belum dijalankan. Layanan lokal port 3001/8080/8001 sudah diperbarui dan merespons sehat; CPU inference menolak frame kosong tanpa membuat identitas. Data operasional pengguna dipertahankan: 1 sekolah, 1 pengguna, 1 siswa, 1 kelas, 1 sesi, dan 1 template wajah. Schema browser sementara tersisa 0.

**Gate staging tersisa:** CI pada SHA yang di-push, pemasangan/restore pada host tujuan, TLS/proxy/kamera, uji beban 500 siswa/dua kiosk, dan evaluasi wajah nyata. Lihat [STAGING-RUNBOOK.md](STAGING-RUNBOOK.md) untuk langkah dan batas aktivasi. Staging administrasi siap menjalani validasi target; otomatisasi wajah belum disetujui.

## Baseline dan cakupan

- `main` berhasil diperbarui dari `f42d356` ke `1052a9d0fb84f6b27c76e687d3b1663cceb86cfb`; pemeriksaan remote terakhir menunjukkan SHA yang sama.
- Pekerjaan lokal sebelumnya dipertahankan, termasuk penghapusan konfigurasi `SCHOOL_ID`. Konflik pull/stash sudah diselesaikan; tidak ada konflik Git tersisa. Snapshot sebelum pull masih ada pada stash `bc7b9a7aec6c8cb14a956bf7a124f7e3c999ba1a`.
- Audit menilai **hasil gabungan main terbaru dan perubahan lokal**, bukan hanya isi commit remote. Perubahan lokal tersebut belum di-commit/push. Container lokal yang sudah berjalan tidak dibangun ulang untuk audit ini.
- Seluruh modul aplikasi dipetakan. Penelusuran mendalam mencakup autentikasi, izin/peran, siswa/import, wajah, sesi/worker, koreksi, laporan/CSV, perangkat/live, akun/pengaturan, konfigurasi, migrasi, deployment, dan CI. Komponen UI hasil generator tidak dibaca satu per satu.
- Probe yang menulis data memakai schema PostgreSQL sementara dengan siswa/akun/template sintetis. Schema itu sudah dihapus. Pemeriksaan akhir data utama: 1 sekolah, 1 pengguna, 0 siswa, 0 kelas, 0 sesi, 0 template wajah; tidak ada schema probe tersisa.

## Must fix — sebelum staging untuk uji pengguna

### 1. [P1] Akses data wajah melewati pembatasan kelas

- **Fungsi:** peran dengan lingkup `assigned_classes` seharusnya hanya mengakses siswa pada kelas tugasnya.
- **Masalah:** daftar wajah hanya menyaring sekolah; status wajah dan helper siswa juga tidak memeriksa kelas. Probe dengan **nol penugasan kelas** mendapat 0 siswa dari endpoint siswa biasa, tetapi 2 siswa dari daftar biometrik, HTTP 200 untuk status wajah siswa lain, dan HTTP 200 untuk penghapusan template sintetisnya. Pemeriksaan di server wajib dilakukan; menyembunyikan menu tidak cukup.
- **Perbaikan minimum:** terapkan pembatasan kelas yang sama pada list/status/preview/enroll/revoke. Bila layanan wajah memang khusus petugas seluruh sekolah, tolak pemberian izin biometrik untuk lingkup kelas dan migrasikan kombinasi peran yang sudah ada. Tambahkan uji negatif untuk kelas lain dan tanpa penugasan.
- **Jika dilewati:** akun terbatas bisa membaca metadata wajah dan menghapus pendaftaran siswa di luar wewenangnya.

Lokasi: `services/api/internal/masterdata/biometrics.go:18`, `:40`, `:57`, `:192`, `:222`; `services/api/internal/auth/policy.go:105`.

### 2. [P1] Pembatasan login menggabungkan semua pengguna di belakang proxy

- **Fungsi:** pembatasan percobaan login mengurangi tebakan password dan beban hashing.
- **Masalah:** kunci IP memakai `RemoteAddr` langsung. Di belakang satu reverse proxy, banyak pengguna terlihat berasal dari alamat proxy yang sama. Probe 30 permintaan login berformat salah melalui peer yang sama, dengan 30 alamat forwarded berbeda, membuat permintaan pengguna ke-31 mendapat HTTP 429. Kuota juga menghitung login berhasil; masa bucket 15 menit.
- **Perbaikan minimum:** tetapkan proxy yang dipercaya secara eksplisit dan ambil alamat klien hanya dari rantai proxy tersebut, atau terapkan kuota per IP klien pada proxy dengan batas aplikasi yang sesuai. Pertahankan perlindungan per akun; jangan mempercayai `X-Forwarded-For` dari sembarang pengirim. Uji beberapa klien melalui proxy staging.
- **Jika dilewati:** beberapa pengguna atau satu pengirim dapat menghabiskan kuota login seluruh sekolah.

Lokasi: `services/api/internal/auth/sessions.go:108`; `services/api/internal/auth/limiter.go:34`.

### 3. [P1] Sesi yang sudah dijadwalkan tetap dapat dibuka pada hari yang kemudian diliburkan

- **Fungsi:** kalender sekolah menentukan apakah sesi boleh berjalan pada tanggal tertentu.
- **Masalah:** pembuatan sesi memeriksa kalender, tetapi pembukaan sesi hanya memeriksa kelas/tahun ajaran aktif. Join kalender pada pembukaan manual dan worker tidak dipakai dalam syarat kelayakan. Probe: buat sesi terjadwal → tandai tanggalnya libur → buka sesi; hasilnya HTTP 200 dan status `open`.
- **Perbaikan minimum:** periksa kalender efektif saat membuka sesi terjadwal, baik melalui API maupun worker, atau batalkan sesi terjadwal ketika tanggalnya diliburkan. Tetapkan perilaku untuk sesi yang sudah terbuka tanpa mengubah riwayat diam-diam. Tambahkan uji perubahan kalender setelah sesi dibuat.
- **Jika dilewati:** sesi berjalan pada hari libur dan dapat menghasilkan catatan ketidakhadiran yang keliru.

Lokasi: `services/api/internal/attendance/schedules.go:212`; `services/api/internal/attendance/sessions.go:165`; `services/api/internal/attendance/worker.go:56`.

### 4. [P1] Penyetuju dengan akses kelas tidak bisa memutuskan koreksi pengguna lain

- **Fungsi:** petugas berizin `attendance.approve` menyetujui atau menolak koreksi pada kelas yang boleh diaksesnya; pengajuan sendiri tetap tidak boleh disetujui sendiri.
- **Masalah:** query koreksi membatasi semua pengguna berlingkup kelas pada pengajuan miliknya sendiri, termasuk pada endpoint keputusan. Probe membuktikan petugas bisa membaca sesi kelas tugasnya (HTTP 200), tetapi keputusan atas pengajuan orang lain dalam kelas itu menghasilkan HTTP 404. Pengajuan sendiri kemudian tertolak oleh aturan larangan persetujuan sendiri.
- **Perbaikan minimum:** pisahkan syarat “hanya pengajuan sendiri” dari pembatasan kelas. Penyetuju dapat melihat/memutuskan pengajuan dalam kelas tugasnya; larangan persetujuan sendiri tetap berlaku. Uji tiga kasus: pengaju lain di kelas tugas, kelas lain, dan pengajuan sendiri.
- **Jika dilewati:** peran penyetuju kelas tidak dapat menjalankan tugasnya.

Lokasi: `services/api/internal/attendance/corrections.go:14`, `:32`, `:166`, `:171`.

### 5. [P1] Browser tidak dapat menghapus foto profil atau logo melalui API lintas origin

- **Fungsi:** tombol hapus gambar mengirim `DELETE` dari aplikasi web ke API.
- **Masalah:** respons preflight CORS tidak menyertakan `DELETE` pada metode yang diizinkan. Probe `OPTIONS /api/v1/auth/profile/photo` dengan origin lokal sah mendapat HTTP 204, tetapi daftar metodenya hanya `GET, HEAD, POST, PATCH, PUT, OPTIONS`. Browser menghentikan permintaan DELETE sebelum handler gambar dijalankan.
- **Perbaikan minimum:** sertakan DELETE pada metode CORS dan tambahkan uji preflight sesuai metode yang dipakai aplikasi. Uji tombol hapus profil/logo dari browser dengan origin web dan API berbeda.
- **Jika dilewati:** fitur hapus gambar gagal meski upload dan izin akun sudah benar.

Lokasi: `services/api/internal/platform/httpserver/handler.go:62`; `services/api/internal/auth/photos.go:29`.

### 6. [P2] Navigasi dan tombol belum mengikuti izin peran dinamis

- **Fungsi:** menu, halaman, dan tindakan harus sesuai izin efektif pengguna.
- **Masalah:** halaman laporan masih membatasi nama peran lama `super_admin/admin/teacher`. Halaman siswa dan tombol tulisnya memakai `academics.read/write`, sementara API memakai `students.read/write`. Eksekusi middleware aktual dengan peran kustom berizin laporan atau siswa menghasilkan redirect `/admin?forbidden=1` meski izin API terkait diberikan.
- **Perbaikan minimum:** gunakan izin efektif yang sama dengan API pada middleware, menu, dan tombol. Bedakan izin siswa dari akademik; tentukan kebutuhan izin untuk opsi kelas pada formulir dan baca/ekspor laporan. Tambahkan uji peran kustom dengan izin minimum.
- **Jika dilewati:** peran yang dibuat di panel tampak sah tetapi fitur yang diberikan tidak dapat dipakai, atau tombol menampilkan tindakan yang akan ditolak API.

Lokasi: `apps/web/app/middleware/reports.ts:1`; `apps/web/app/middleware/academics.ts:1`; `apps/web/app/composables/useMasterData.ts:6`; `apps/web/app/components/AppSidebar.vue:28`; `services/api/internal/masterdata/students.go:34`.

### 7. [P1, release] Petunjuk instalasi baru menunggu API yang belum bisa hidup

- **Fungsi:** instalasi kosong harus dimigrasikan, dibuatkan sekolah/admin pertama, lalu menjalankan API.
- **Masalah:** startup lokal terbaru menghentikan API bila sekolah belum ada. README deployment langkah 7 justru meminta menunggu API siap lalu menjalankan bootstrap dengan `docker exec` ke container API aktif. Pada database kosong, prasyarat itu tidak tercapai. Runbook lokal sudah memakai urutan bootstrap sebelum API, sehingga kedua petunjuk tidak konsisten.
- **Perbaikan minimum:** dokumentasikan dan buktikan urutan migrasi → bootstrap sekali jalan menggunakan image API dan stdin privat → mulai API → smoke test login. Perintah bootstrap tidak boleh bergantung pada API yang sedang hidup. Uji juga bahwa kegagalan migrasi menahan startup.
- **Jika dilewati:** pemasangan staging pertama macet walaupun ketiga image berhasil dibangun.

Lokasi: `README.md:131`; `services/api/cmd/api/main.go:43`; `services/api/internal/auth/school.go:20`.

### 8. [P1, release] Kode lokal yang diaudit belum menjadi revisi rilis

- **Fungsi:** staging harus dibangun dari revisi yang dapat ditinjau dan diuji ulang.
- **Masalah:** penghapusan SCHOOL_ID, penemuan sekolah otomatis, dan beberapa penyesuaian terkait masih berupa perubahan lokal; dua file auth bahkan belum terlacak Git. Deploy langsung dari main saat ini tidak membawa seluruh perilaku yang dinilai dalam audit ini.
- **Perbaikan minimum:** tinjau dan commit perubahan lokal beserta perbaikan di atas, jalankan CI pada SHA final, lalu gunakan SHA itu sebagai sumber deployment. Catat migrasi dan prosedur rollback untuk revisi yang benar-benar dirilis.
- **Jika dilewati:** hasil pengujian lokal berbeda dari staging dan rollback sulit ditelusuri.

Lokasi: status Git saat audit, khususnya `services/api/internal/auth/school.go` dan `school_integration_test.go` yang belum terlacak. Tidak ada commit/push/deploy yang dilakukan oleh audit ini.

## Should fix — tutup sebelum membuka staging lebih luas

### 9. [P2] Temuan dependency web masih perlu keputusan rilis yang jelas

- **Fungsi:** pemeriksaan dependency membantu menilai paket rentan yang ikut proses build atau runtime.
- **Masalah:** `npm audit --omit=dev` menghasilkan **15 entri paket terdampak: 11 high, 4 moderate**, dengan tiga advisory asal. Itu bukan 15 CVE berbeda. CI membolehkan langkah npm audit gagal, sehingga status CI hijau tidak menyelesaikan temuan ini. Paket asal tersebut tidak ditemukan pada pencarian nama paket dalam output server lokal yang ada; keterpaparan runtime belum terbukti dan output itu belum dibangun ulang pada audit ini.
- **Perbaikan minimum:** telusuri penggunaan saat build dan dalam image runtime final; perbarui dependensi pada versi yang kompatibel bila tersedia, atau dokumentasikan pengecualian berbatas waktu dengan alasan keterjangkauannya. Jangan menjalankan `npm audit fix --force` tanpa menilai perubahan Nuxt yang diusulkan.
- **Jika dilewati:** keputusan rilis tidak membedakan risiko tool build dan endpoint yang benar-benar terpapar.

Sumber: [braces](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm), [node-forge](https://github.com/advisories/GHSA-86w9-cpqp-85rv), [decode-uri-component](https://github.com/advisories/GHSA-vcc3-ghjq-m6fr). Python locked dependency scan bersih. Go tidak menemukan jalur pemanggilan rentan; temuan module-only `GO-2026-5932` terkait paket OpenPGP yang tidak digunakan aplikasi, bukan temuan Argon2 yang dipakai autentikasi.

### 10. [P2] Belum ada pengujian browser menyeluruh untuk batas web–API–proxy

- **Fungsi:** tes unit/integrasi memeriksa aturan aplikasi; tes browser memeriksa alur pengguna melalui browser dan proxy sebenarnya.
- **Masalah:** 53 tes UI yang tersedia berjalan di Node dengan lingkungan tiruan, bukan browser penuh. Tes yang ada lolos sementara masalah CORS dan middleware di atas masih dapat direproduksi.
- **Perbaikan minimum:** tambahkan sedikit tes browser untuk login/logout, akun/foto, peran kelas, siswa/import, koreksi, dan laporan; jalankan smoke test HTTPS/proxy, WebSocket, serta izin kamera di staging. Jadikan kasus temuan 1–7 tes regresi sesuai lapisannya.
- **Jika dilewati:** kegagalan integrasi baru diketahui oleh operator meskipun semua unit test hijau.

Lokasi: `scripts/test_*.mjs`; `.github/workflows/ci.yml`.

## Kesiapan fitur

| Area | Sudah tersedia | Yang masih diperlukan |
|---|---|---|
| Login, sesi akun, profil | Password/hash, cookie/CSRF, perubahan akun, foto | Perbaikan proxy dan CORS; uji cookie Secure melalui domain staging |
| Pengguna/peran | Peran dinamis, permission, penugasan kelas | Konsistensi UI dan pembatasan biometrik/koreksi |
| Akademik/siswa | Tahun ajaran, kelas, penempatan, import CSV dry-run/commit | UAT izin minimum dan fixture sekolah staging; XLSX belum tersedia dan tidak wajib untuk staging CSV |
| Jadwal/kehadiran manual | Kalender, worker, roster, hadir/pulang, rekonsiliasi, koreksi | Aturan perubahan hari libur dan penyetuju kelas |
| Ringkasan/laporan | Chart, filter, pagination, CSV dengan perlindungan formula | Akses peran kustom dan uji unduhan di browser staging |
| Branding/tampilan | Nama/logo sekolah, tema terang/gelap/sistem | Uji visual/responsif dan perubahan gambar melalui domain staging |
| Perangkat/kiosk | Pairing, heartbeat, sesi perangkat, live invalidation | Uji putus jaringan, habis sesi, dua perangkat, dan proxy WebSocket |
| Pendaftaran wajah | Kamera/preview, 3–5 frame, kualitas/konsistensi, template terenkripsi, hapus data | Perbaikan akses; pemeriksaan satu wajah didaftarkan sebagai siswa berbeda; uji kamera nyata |
| Identifikasi wajah 1:N | Modul matcher/tracking offline dan tes; gate server menolak hasil belum disetujui | Sambungkan engine ke endpoint nyata, pemeriksaan anti foto/video, evaluasi hold-out, kalibrasi, dan uji beban |
| Deployment | Tiga Dockerfile, migrasi sekali jalan, volume model persisten, healthcheck, CI build image | Bootstrap instalasi kosong, least-privilege DB, domain/TLS, restore dan smoke test pada Coolify target |

### Batas fitur wajah otomatis

`services/ai/app/main.py:179` memanggil `pipeline.recognize(body.image)`; jalur itu hanya mendeteksi kotak dan menghasilkan `unknown/inconclusive/no_face`. Gallery belum dipakai untuk pencocokan identitas di endpoint ini. `pipeline.py:306` dan status layanan selalu menetapkan `recognition_enabled=False`. Modul `face_engine.py` sudah mempunyai bagian pencocokan dan tracking, tetapi belum tersambung ke route tersebut.

Model deteksi dan embedding beserta manifest/checksum/lisensinya sudah tersedia. Yang belum ada ialah implementasi pemeriksaan keaslian wajah (menolak foto/video), sambungan hasil identitas ke alur kiosk, deteksi pendaftaran ganda lintas siswa, serta bukti akurasi kamera. Pemeriksaan frame duplikat dan konsistensi dalam satu pendaftaran sudah ada; itu tidak menggantikan pemeriksaan antar-siswa.

Evaluator `services/ai/app/evaluation.py:9` merangkum trace keputusan yang sudah dibuat, dan selalu mengembalikan `automatic_attendance_approved=False`. Ini belum merupakan runner evaluasi kamera atau mekanisme aktivasi. API Go juga memeriksa bukti evaluasi di `services/api/internal/attendance/recognize.go:27`; pertahankan gate tersebut sampai integrasi dan evaluasi benar-benar selesai.

Dataset uji terpisah belum diberikan sesuai jawaban pemilik proyek sebelumnya. Karena itu angka akurasi, ambang kecocokan, tingkat salah identifikasi, ketahanan foto/video, dan latensi pada 500 siswa **belum dapat dinyatakan**. Pendaftaran wajah dapat diuji terbatas di staging sesudah akses diperbaiki; pencatatan otomatis tetap nonaktif.

Keputusan pemilik proyek untuk menghapus alur wali/persetujuan sudah tercermin pada migrasi 11 dan alur aplikasi. Audit ini tidak meminta fitur itu dikembalikan. Selaraskan bagian PRD/arsitektur lama yang masih menjelaskan alur yang telah dihentikan dengan ruang lingkup aplikasi sekarang.

## Bukti verifikasi

| Pemeriksaan | Hasil |
|---|---|
| Git | Main lokal = origin/main `1052a9d`; `git diff --check` bersih; pekerjaan lokal tersimpan |
| Go | `go test -race ./...` dengan database uji terisolasi lolos; beberapa paket memakai cache tes; `go vet ./...` dan gofmt lolos |
| Web | ESLint, pemeriksaan format aplikasi, dan Nuxt typecheck lolos; 53 tes Node lolos |
| AI | Ruff check/format pada direktori layanan lolos; 66 pytest lolos |
| Skrip | 5 unittest Python lolos |
| Dependency | Go tanpa temuan pemanggilan rentan; Python tidak ada kerentanan diketahui; npm memerlukan triase pada temuan 9 |
| CI remote | [Run untuk main](https://github.com/Arseno25/attendance-web/actions/runs/38676863445) sukses, termasuk build image web/API/AI; langkah npm audit bersifat nonblocking |
| Probe regresi terpisah | Bypass kelas wajah, koreksi, kalender libur, dan kuota proxy berhasil direproduksi; preflight DELETE dan middleware diverifikasi terpisah |
| Data utama setelah audit | Tetap 1 sekolah/1 pengguna dan tidak ada data operasional; schema probe tersisa 0 |

Bukti mesin lokal tersimpan pada `.tools/staging-audit-2026-10-15/`: log Go/UI, JSON hasil probe, hasil pemindaian dependency, serta metadata CI. Folder itu diabaikan Git; laporan ini memuat hasil penting agar dapat ditinjau tanpa folder privat tersebut.

## Urutan menuju staging

1. **Perbaiki aturan dan akses.** Tutup temuan 1–6 dengan tes regresi kecil. Utamakan pembatasan wajah, login melalui proxy, kalender, dan persetujuan koreksi.
2. **Siapkan revisi rilis yang dapat dipasang dari kosong.** Selesaikan bootstrap/doc dan perubahan lokal pada temuan 7–8. Triage dependency, jalankan seluruh pemeriksaan wajib serta CI pada SHA final; catat revisi image dan migrasi. Jangan membawa akun/password dummy development ke staging yang terbuka.
3. **Pasang staging administrasi dengan data dummy.** Konfigurasikan web/API HTTPS pada susunan domain yang sesuai cookie; origin harus tepat. Hubungkan database privat dengan runtime role berhak minimum dan migrator terpisah. Uji migrasi gagal menahan API, model volume kosong dapat diisi, redeploy mempertahankan volume, bootstrap sekali jalan, health, dan login.
4. **Uji alur operator melalui browser dan proxy.** Uji tiap peran, siswa/import, jadwal termasuk hari libur, hadir/pulang/koreksi, chart/laporan, foto/logo/tema, audit, pairing/expiry, dan WebSocket. Uji penolakan izin kamera, koneksi terputus, serta pesan error yang dapat dipahami.
5. **Tutup kebutuhan operasional.** Backup database dan kunci enkripsi ke lokasi terpisah, jadwalkan backup, uji restore di target staging, tentukan toleransi kehilangan data/waktu pemulihan, dan pasang pemantauan health/error serta kapasitas disk. Restore lokal sebelumnya sudah diuji; itu belum membuktikan restore pada host Coolify. Ukur beban 500 siswa/dua kiosk sebelum menaikkan beban; jangan menambah replika API sebelum state kuota/worker dievaluasi.
6. **Selesaikan pilot wajah sebagai tahap terpisah.** Hubungkan engine, pemeriksaan keaslian dan pendaftaran ganda; buat runner evaluasi dengan sampel terpisah serta skenario orang tak dikenal/foto/video. Kalibrasikan ambang/margin dan ukur salah identifikasi, gagal dikenali, ketahanan serangan, serta latensi. Aktifkan pencatatan otomatis hanya setelah bukti sesuai model/kamera yang dipakai ditinjau.

**Kriteria lulus staging administrasi:** temuan wajib selesai, revisi final CI hijau dengan keputusan dependency tercatat, instalasi kosong berhasil, smoke test seluruh peran/alur lulus, dan restore target terbukti. Identifikasi otomatis tetap nonaktif sampai tahap 6 lulus.

**Verdict:** fondasi administrasi sudah luas dan tes dasarnya berjalan; perbaiki batas akses dan aturan operasional sebelum uji pengguna staging. Fitur wajah otomatis masih membutuhkan implementasi, bukan sekadar konfigurasi deployment.

**Not checked:** deployment Coolify/domain/TLS sebenarnya, kamera fisik, akurasi wajah nyata, pengujian beban, dan isi setiap komponen UI generator. Audit ini tidak melakukan deploy, commit, push, atau memperbaiki temuan; perubahan kode yang terjadi hanya untuk menyatukan pull dengan pekerjaan lokal yang sudah ada.
