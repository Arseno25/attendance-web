# Audit keamanan, shadcn-vue, dan responsivitas

Tanggal: 15 Oktober 2026, Asia/Jakarta. Baseline: `8773abf`, branch `codex/staging-readiness`.

> **Pembaruan 16 Oktober 2026:** bagian audit awal di bawah merekam baseline sebelum perbaikan. Lihat [hasil remediasi](#hasil-remediasi--16-oktober-2026) untuk status kode dan batas verifikasi terbaru.

## Kesimpulan

Fondasi UI konsisten dengan shadcn-vue: konfigurasi new-york/neutral, shell dashboard-01, login-04, primitive Reka, token tema, tabel TanStack, dialog, pagination, serta notifikasi Sonner. Komponen produk yang reusable seperti kamera dan toolbar merupakan komposisi yang wajar; shadcn tidak mengharuskan setiap halaman identik dengan contoh block.

Namun, **belum layak dinyatakan siap staging untuk akses banyak peran**. Ada tiga kelompok masalah otorisasi yang berhasil direproduksi: batas kelas pada mutasi master data, batas kelas saat pairing, dan pencabutan izin operator yang tidak mencabut sesi perangkat. Ada pula risiko pengiriman ulang foto yang terbukti pada protokol sintetis, dengan pengujian model/perangkat nyata masih diperlukan.

Audit ini tidak mengubah kode aplikasi, konfigurasi, model, ataupun data sekolah yang ada. Probe menulis hanya ke schema PostgreSQL sementara yang dihapus melalui cleanup. Browser otomatis memakai API nyata dan data sintetis terisolasi; kamera tidak diaktifkan dan foto wajah pribadi tidak direkam. Perubahan repository hanya laporan dan bukti UI sintetis.

## Temuan keamanan terkonfirmasi

### S1 — P1 / High: cakupan kelas tidak diterapkan pada beberapa mutasi master data

**Lokasi:** `services/api/internal/masterdata/students.go:184`, `students.go:237`, `imports.go:84`, `imports.go:166`, `assignments.go:51`, `assignments.go:93`.

`classBounds()` memeriksa sekolah, kelas aktif, dan tahun aktif, tetapi mengabaikan `sc.classScoped`/`sc.classes`. Handler penempatan membaca siswa dengan `school_id` dan `id` tanpa `studentScope`. Handler penugasan juga memakai helper tersebut tanpa memeriksa kelas target terhadap izin aktor.

**Prasyarat:** akun aktif dengan custom role `assigned_classes` dan izin `students.write`, atau `academics.write` untuk penugasan. Pelaku perlu mengetahui UUID siswa/kelas target; ini bukan bypass login atau akses lintas sekolah.

**Bukti HTTP + database terisolasi:**

| Skenario | Hasil aktual |
| --- | --- |
| Pengelola kelas A membaca siswa kelas B | GET detail ditolak, HTTP 404 |
| Pengelola yang sama memindahkan siswa B ke A melalui endpoint placements | HTTP 201; GET siswa kemudian menjadi HTTP 200 |
| Pengelola kelas A mengimpor CSV ke kelas B | Preview HTTP 201, commit HTTP 200; satu penempatan di kelas B benar-benar tersimpan |
| Peran dengan `academics.write`, tetap scope kelas A, menambahkan penugasan dirinya ke B | HTTP 201; `/auth/me` kemudian mengembalikan kedua kelas |

**Dampak:** perubahan penempatan siswa di luar kewenangan, penambahan siswa ke kelas lain, dan perluasan akses kelas oleh pemegang izin akademik terbatas. Filter dropdown frontend tidak melindungi API.

**Perbaikan:** terapkan pemeriksaan kelas pada helper bersama sebelum mutasi; periksa siswa sumber dan kelas tujuan pada perpindahan. Atur operasi penugasan agar aktor tidak dapat memperluas cakupannya sendiri. Tinjau pencabutan assignment (`assignments.go:130`) karena query target juga hanya membatasi sekolah; cabang ini ditemukan melalui source dan belum diprobe terpisah. Tambahkan regresi HTTP untuk siswa/kelas asing, termasuk hasil database tidak berubah saat ditolak.

### S2 — P1 / High: operator terbatas kelas bisa memasangkan sesi kelas lain

**Lokasi:** `services/api/internal/attendance/devices.go:147`, `devices.go:189`.

`kioskOptions()` membuang user hasil otorisasi lalu mengembalikan seluruh sesi sekolah yang terbuka. `kioskSession()` memvalidasi sesi dan token pairing, tetapi tidak memeriksa kelas sesi terhadap scope aktor.

**Prasyarat:** akun aktif dengan `devices.health` atau izin pengelolaan perangkat yang mengimplikasikannya, scope `assigned_classes`, dan token pairing sah yang diterbitkan pengelola. UUID/token tidak ditebak atau dibocorkan oleh probe.

**Bukti:** akun custom tanpa satu pun penugasan kelas mendapat sesi kelas BIO dari GET session-options, kemudian POST kiosk/session menghasilkan HTTP 201 dengan token pairing sah.

**Dampak:** sesi perangkat dapat diarahkan ke kelas di luar izin operator, sehingga cakupan data pengenalan/absensi perangkat melewati pembatasan kelas.

**Perbaikan:** gunakan filter kelas yang sama pada daftar pilihan dan query pairing. Token yang salah atau sesi di luar scope harus ditolak secara transaksional. Jika perangkat memang hanya boleh dikelola pada scope sekolah, validasi pembatasan itu secara eksplisit saat menetapkan role dan pada endpoint, bukan membiarkan kombinasi izin/scope yang tidak ditegakkan.

### S3 — P2 / Medium: sesi perangkat tetap dapat menulis setelah izin operator dicabut

**Lokasi:** `services/api/internal/auth/roles.go:288`, `services/api/internal/attendance/devices.go:254`, `services/api/internal/attendance/recognize.go:170`.

Perubahan role menghapus `user_sessions`, tetapi tidak `device_sessions`. Kiosk memeriksa masa berlaku, perangkat/sesi/sekolah aktif. Jalur penerimaan pengenalan memeriksa `u.is_active`, tetapi tidak izin atau cakupan kelas operator saat ini.

**Prasyarat:** kiosk sudah dipasangkan sebelum pencabutan; akun operator masih aktif; perangkat dan sesi tetap terbuka. Masa paparan dibatasi sisa masa berlaku perangkat, maksimal sepuluh menit dari pairing.

**Bukti:** setelah permission role operator diubah menjadi kosong melalui PATCH roles, request pengenalan dari cookie perangkat lama tetap HTTP 200 dengan outcome `recorded`; database berisi tepat satu record baru. AI pada probe ini adalah service sintetis yang mengembalikan hasil valid, sehingga uji mengisolasi otorisasi API.

**Dampak:** pencabutan izin tidak langsung menghentikan pencatatan absensi dari perangkat yang sebelumnya diterbitkan akun itu.

**Perbaikan:** cabut sesi perangkat pada perubahan role/izin/penugasan relevan dan validasi izin operator saat menggunakan credential perangkat. Pertahankan pencabutan sesi perangkat yang sudah ada pada revoke perangkat. Tambahkan uji untuk role berubah, permission dicabut, assignment berakhir, dan akun nonaktif; jangan mengandalkan logout panel saja.

## Risiko dan pemeriksaan keamanan yang belum tuntas

### S4 — Prioritas P1: protokol belum membuktikan kesegaran foto kamera

**Lokasi:** `services/api/internal/attendance/recognize.go:61`, `services/ai/app/recognition.py:25`, `services/ai/tests/test_recognition_service.py:40`.

Timestamp pengambilan dibuat klien dan hanya diperiksa dalam rentang 30 detik. Event UUID dapat diganti. AI tidak mengharuskan frame berbeda untuk akumulasi kecocokan; ledger mengamankan idempotensi event, bukan asal atau kesegaran gambar.

Tes HTTP sintetis yang dijalankan ulang mengirim **JPEG identik lima kali** dengan event UUID baru dan memperoleh urutan `PROCESSING, PROCESSING, CONFIRMED, CONFIRMED, CONFIRMED`. Pipeline dan PAD pada tes tersebut adalah stub. Ini membuktikan celah kesegaran protokol, **bukan bukti bahwa foto cetak/video berhasil menipu model nyata**.

Serangan praktis memerlukan akses ke browser/perangkat yang sudah dipasangkan serta JPEG siswa peserta yang sebelumnya bisa lolos PAD dan pencocokan. Foto tersebut berpotensi dikirim ulang dengan metadata baru tanpa siswa hadir. Belum dilakukan pengujian ini pada model nyata karena audit tidak mengambil atau menyimpan foto pribadi tambahan.

Tindak lanjut: rancang proteksi kesegaran yang mencakup batas perangkat tepercaya dan pemeriksaan liveness pada urutan tangkapan/challenge, lalu uji rekaman/foto/replay pada perangkat nyata. Dedup hash berguna sebagai lapisan awal, tetapi tidak cukup menghadapi foto yang dikompres ulang atau video. Jangan menurunkan threshold agar tes terlihat lolos.

### Hardening dan kapasitas yang perlu diverifikasi sebelum staging

| Area | Bukti / batas pemeriksaan | Tindakan |
| --- | --- | --- |
| Header web | Respons lokal `/login` tidak memiliki CSP, `frame-ancestors`/X-Frame-Options, atau Referrer-Policy; API memiliki nosniff/no-store. Konfigurasi reverse proxy staging belum diperiksa. | Verifikasi header pada origin HTTPS sebenarnya; tambahkan kebijakan frame dan CSP yang cocok dengan Nuxt, chart, kamera, dan API. Belum ada eksploit clickjacking/XSS yang dibuktikan. |
| Kontensi saat pengenalan | `kioskAuth(write=true)` mengambil `FOR UPDATE` pada baris sekolah sebelum panggilan AI; transaksi tetap terbuka saat inferensi. Jalur lain juga memakai lock sekolah. | Uji banyak perangkat bersamaan dan perlambatan AI. Pendekkan transaksi inferensi sambil tetap memvalidasi ulang izin, galeri, sesi, dan idempotensi sebelum commit. Audit ini tidak menjalankan load test. |
| Rate limit | Login memiliki batas akun/IP dan hashing semaphore; limiter proses tunggal. | Untuk deployment dengan beberapa replica, pastikan batas di proxy/shared store sesuai rancangan. Tidak dinilai sebagai bypass pada deployment lokal satu proses. |
| Operasional | Dockerfile runtime nonroot, model read-only, token internal; konfigurasi contoh memisahkan URL migrator/runtime. | Verifikasi grant database, TLS, secret staging, backup/restore, serta CVE image OS pada lingkungan target. File contoh bukan bukti izin DB aktual. |
| Dokumentasi | Komentar compose dan beberapa catatan lama masih menyebut gate offline yang berbeda dari alur runtime sekarang. | Perbarui penjelasan agar operator tidak menyamakan model terpasang dengan akurasi/liveness yang telah dievaluasi. Jangan mengubah perilaku hanya untuk menyesuaikan komentar lama. |

## Dependensi dan kontrol yang sudah baik

| Pemeriksaan baru | Hasil dan interpretasi |
| --- | --- |
| `govulncheck -json ./...` | Exit 0; tidak ada simbol rentan yang dilaporkan terjangkau. Ada satu temuan tingkat modul, GO-2026-5932, untuk paket OpenPGP yang tidak diimpor aplikasi. Aplikasi memakai Argon2 dari modul yang sama. |
| `pip-audit` runtime lock | 37 dependensi, tidak ada advisory yang dilaporkan. |
| `npm audit --json` | 19 entri paket terdampak: 14 high, 5 moderate, berasal dari tiga advisory sumber. Ini bukan status dependensi bersih dan bukan 19 celah aplikasi yang telah dieksploitasi. |
| Gate dependensi web | Tiga advisory build tooling sesuai pengecualian repository; paket terdampak tidak ditemukan dalam metadata output runtime yang diperiksa gate. Pengecualian berakhir 15 November 2026. Pemeriksaan metadata bukan pengganti SBOM/image scan menyeluruh. |
| Gitleaks Git | 26 commit dipindai, tidak ada kebocoran secret yang terdeteksi. Tidak mencakup seluruh file ignored atau secret di lingkungan staging. |

Advisory sumber yang diverifikasi: [braces](https://github.com/advisories/GHSA-vfj7-8cjw-p6xm), [node-forge](https://github.com/advisories/GHSA-86w9-cpqp-85rv), [decode-uri-component](https://github.com/advisories/GHSA-vcc3-ghjq-m6fr), dan [Go OpenPGP](https://pkg.go.dev/vuln/GO-2026-5932). Ketiga advisory npm berada pada dependency tree tooling yang sudah diberi pengecualian; jangan menjalankan `npm audit fix --force` karena saran versi npm tidak menjamin kompatibilitas Nuxt/shadcn.

Kontrol yang dipertahankan: Argon2id dengan salt acak dan perbandingan konstan; cookie HttpOnly/SameSite serta Secure pada origin HTTPS; sesi disimpan sebagai hash; validasi Origin + CSRF pada mutasi; reload izin dan sesi pada transaksi panel; SQL berparameter; batas ukuran/format JSON dan unggahan; re-encode foto JPG/PNG; AES-GCM template dengan nonce acak dan identitas sebagai associated data; checksum model; service token AI; penyaringan identitas pada preview/hasil unknown; perlindungan formula CSV; audit/event append-only; koreksi oleh petugas berbeda; idempotensi absensi dan timestamp server.

Kontrol tersebut tidak menutup temuan scope di atas. Tidak ditemukan jalur SQL injection, XSS berbasis `v-html`, atau akses template plaintext dari endpoint publik pada source yang ditinjau; ini bukan jaminan bebas kerentanan.

## Audit UI dan shadcn-vue

**Implementation integrity: lulus sebagai sistem shadcn-vue yang koheren, dengan penyimpangan lokal yang perlu dibenahi.** Detector Impeccable tidak melaporkan temuan deterministik; pemeriksaan browser tetap menemukan masalah layout, sehingga hasil detector tidak dipakai sebagai bukti bahwa seluruh UI sudah benar.

Dokumentasi dikonsultasikan melalui Context7 `/websites/shadcn-vue` dan MCP registry untuk dashboard-01, login-04, Dialog, sidebar dan pola tabel. Tidak memasang ulang atau menimpa source component saat audit.

| Bagian | Hasil |
| --- | --- |
| Konfigurasi | `components.json`: new-york, neutral, CSS variables, alias Nuxt; main.css mengimpor Tailwind dan CSS shadcn; font Inter lokal. |
| Shell admin | SidebarProvider → AppSidebar inset → SidebarInset → SiteHeader; kategori menu dan akses menggunakan komponen Sidebar. |
| Login | Struktur login-04, Card/Field/Input/Button; validasi inline dan pesan autentikasi di form. |
| Tabel | DataTable reusable berbasis TanStack + Table; caption, header kolom, loading/empty state; scroll internal. Tabel lebar bukan overflow halaman dengan sendirinya. |
| Filter | Toolbar tabel memisahkan search dan dropdown; filter laporan/audit berupa grid tingkat halaman. Tidak ada filter di overview. |
| Aksi tabel | Mayoritas menggunakan TableActions: satu/dua ikon dengan tooltip, tiga atau lebih dropdown. Halaman Peran masih menyimpang, lihat U2. |
| Pagination | TablePagination mengomposisikan Pagination shadcn dengan total dan halaman server; tidak memotong ulang data yang sudah dipaginasi API. |
| Form | Input/Select/Checkbox/Switch/Textarea/FieldError serta schema Zod. Tidak ditemukan native button/select/table pengganti di halaman/komponen produk pada inventaris source. |
| Overlay | Form produk memakai Dialog/AlertDialog. Sheet yang tersisa adalah sidebar mobile, sesuai fungsi komponen Sidebar resmi. Dua modal panjang diperiksa di browser. |
| Status/tema | Badge memakai mapping semantic success/warning/info/destructive/secondary; warna memiliki pasangan foreground untuk light/dark. Preferensi system mengikuti media browser pada sampel. |
| Kamera dan unggahan | Komponen produk reusable mengomposisikan primitive shadcn dengan API kamera/canvas/file. Bagian ini memang membutuhkan elemen native; tidak perlu mengganti kamera dengan komponen dekoratif. |

Inventaris: 20 file page, 19 komponen produk, dan 29 kelompok primitive UI. Kelompok primitive: alert, alert-dialog, avatar, badge, breadcrumb, button, card, chart, checkbox, dialog, dropdown-menu, empty, field, input, label, pagination, progress, select, separator, sheet, sidebar, skeleton, sonner, spinner, switch, table, tabs, textarea, tooltip.

### U1 — P1: kelompok tombol header melebar di mobile

**Lokasi:** `apps/web/app/components/PageHeader.vue:18`, `pages/admin/students/[id].vue:93`, `pages/admin/devices.vue:103`. Kategori: Responsive / Layout.

Container actions menggunakan `shrink-0` tanpa batas lebar; beberapa caller menambahkan grup flex yang mempertahankan lebar intrinsiknya. Posisi kanan atas benar pada desktop, tetapi grup tidak menyesuaikan lebar layar kecil.

| Halaman | Viewport | Lebar dokumen aktual |
| --- | ---: | ---: |
| Detail siswa | 390 px | 514 px |
| Detail siswa | 320 px | 514 px |
| Perangkat | 320 px | 365 px |

Tombol penempatan/tambah perangkat bergeser ke luar tampilan; pengguna harus menggeser seluruh halaman. Perbaiki batas lebar dan wrapping pada PageHeader bersama beserta grup bersarang; tetap rata kanan jika ruang cukup. Jangan memotong aksi dengan `overflow-hidden`. Regresi harus memeriksa `scrollWidth <= innerWidth` dan seluruh tombol tetap dapat digunakan, termasuk nama siswa panjang.

Bukti sintetis: [detail siswa 390 px](audit-assets/2026-10-15/student-detail-390.png), [perangkat 320 px](audit-assets/2026-10-15/devices-320.png).

### U2 — P2: tindakan tabel Peran belum mengikuti pola bersama

**Lokasi:** `apps/web/app/pages/admin/roles.vue:205`. Kategori: Implementation integrity.

Baris memakai tombol teks Edit/Hapus, sementara permintaan produk menetapkan satu/dua aksi sebagai ikon dengan tooltip melalui TableActions. Tombol merah berulang juga lebih dominan daripada data role. Komponennya tetap Button shadcn, jadi ini penyimpangan komposisi, bukan pemakaian library yang salah.

Gunakan TableActions yang sudah ada, pertahankan pembatasan role sistem serta dialog konfirmasi hapus. Tidak perlu wrapper/dependency baru.

### U3 — P2: label tanggal grafik berdesakan pada 320 px

**Lokasi:** `apps/web/app/components/AttendanceTrendChart.vue:87`. Kategori: Responsive / Readability.

Seluruh tujuh tick tanggal selalu dirender dengan format hari-bulan dan alokasi teks 40 px. Pada card sempit, label terlihat menyambung sehingga hari sulit dibedakan. Grafik dan halaman tidak overflow, tetapi isi grafik kurang terbaca.

Kurangi jumlah tick atau pendekkan label sesuai lebar chart; tooltip tetap memuat tanggal lengkap. Pertahankan ringkasan teks pembaca layar dan data laporan. Header card juga perlu memberi ruang agar judul/deskripsi tidak terlalu sempit di sebelah tombol laporan.

Bukti sintetis: [overview 320 px](audit-assets/2026-10-15/overview-320.png).

### U4 — P2: beranda publik memberi informasi fitur yang sudah tidak sesuai

**Lokasi:** `apps/web/app/pages/index.vue:73`. Kategori: Implementation integrity / UX copy.

Kalimat “Absensi wajah belum tersedia” selalu tampil, walaupun alur absensi kamera sudah berfungsi pada instalasi lokal. Ini dapat membuat siswa/guru mengira fitur belum bisa dipakai. Gunakan petunjuk stabil menuju alur yang tersedia atau status layanan yang benar; jangan menyamakan health check API dengan akurasi pengenalan.

### Ringkasan posisi dan responsivitas seluruh halaman

Matriks berikut mengacu pada browser otomatis dengan fixture sintetis: 1440×1000 light, 1024×768 light, 768×1024 dark, 390×844 dark, 320×740 light. Total **100 kombinasi halaman/viewport**. Tidak ada page overflow selain U1. Tabel tetap boleh scroll di containernya. Bukan semua kombinasi light/dark pada setiap lebar.

| Halaman | Posisi/layout yang ditinjau | Hasil |
| --- | --- | --- |
| `/` | Header, tombol masuk, status layanan | Muat di lima lebar; copy U4 |
| `/login` | Card login-04, field dan tombol | Muat; panel gambar tersembunyi pada mobile |
| `/absensi` | Halaman mandiri, form pairing | Muat; kamera aktif tidak diuji ulang dalam audit ini |
| `/admin` | Statistik, grafik, akses operasional | Muat; label grafik U3 |
| `/admin/academics` | Header, tabs, tabel tahun ajaran | Muat; tab lain diperiksa melalui source |
| `/admin/students` | Header, search, filter kanan, tabel, pagination | Muat |
| `/admin/students/:id` | Identitas, histori, tombol tindakan | Overflow 320/390, U1 |
| `/admin/students/import` | File/kelas, tombol tinjau/contoh | Muat; preview berisi data tidak dibuka pada audit layout |
| `/admin/biometrics` | Search, tabel, tombol kelola | Muat; tidak mengaktifkan kamera |
| `/admin/schedules` | Header, tabs, tabel jadwal | Muat; kalender juga diamati pada browser pengguna |
| `/admin/attendance` | Tanggal, tombol buat sesi, tabel | Muat |
| `/admin/attendance/:id` | Status sesi, roster, filter, aksi | Muat pada sesi fixture terbuka |
| `/admin/corrections` | Filter dan tabel koreksi pending | Muat |
| `/admin/reports` | Ekspor kanan, grid filter, rekap/rincian | Muat; tabel bergulir internal |
| `/admin/devices` | Dua tombol header, tabel kosong | Overflow pada 320, U1 |
| `/admin/users` | Tombol tambah, search, tabel | Muat |
| `/admin/roles` | Tombol tambah dan aksi tiap baris | Muat; aksi tidak seragam U2 |
| `/admin/audit` | Grid filter halaman, tabel | Muat |
| `/admin/account` | Simpan kanan, foto/profil/password | Muat; kolom bertumpuk pada mobile |
| `/admin/settings` | Simpan kanan, tab Sekolah/Tampilan | Tab Sekolah muat; komponen Tampilan dicek melalui source dan system media pada halaman |

Modal Tambah jadwal pada 390×844 memiliki batas x=16, y=16, lebar 358, tinggi 812 px; konten panjang bergulir dalam dialog. Modal Tambah siswa berada pada x=16, y=111, lebar 358, tinggi 622 px. Judul, deskripsi, label, tombol batal/simpan tersedia dalam accessibility tree. Screenshot awal yang menangkap animasi diganti dengan screenshot setelah animasi diselesaikan; tidak dihitung sebagai bug transparansi modal.

Pada 95 sampel halaman dengan inventaris kontrol, tidak ditemukan tombol terlihat tanpa nama atau input terlihat tanpa label oleh pemeriksaan DOM sederhana. Ini bukan audit aksesibilitas penuh: zoom 200%, keyboard virtual, screen reader, seluruh focus order dan kontras setiap state belum diperiksa. Tombol ikon 32 px mengikuti komponen yang ada, tetapi target sentuh lebih besar dapat dipertimbangkan untuk kiosk/mobile; ukuran 44 px bukan otomatis syarat kelulusan semua kontrol WCAG AA.

### Audit health score UI

| Dimensi | Nilai / 4 | Dasar |
| --- | ---: | --- |
| Accessibility | 3 | Label/landmark/dialog tersedia pada sampel; evaluasi assistive technology dan seluruh state belum lengkap |
| Performance | 3 | Pagination server, inferensi tidak tumpang tindih di UI, galeri bounded; belum ada benchmark perangkat/traffic sekolah |
| Theming | 3 | Token semantik konsisten dan light/dark/system bekerja pada sampel; seluruh kontras state belum diukur |
| Responsive | 2 | Dua halaman overflow pada mobile; tick grafik terlalu padat |
| Implementation integrity | 3 | Komposisi shadcn koheren; satu tabel menyimpang dan copy beranda tertinggal |
| **Total** | **14/20** | **Good dengan perbaikan terarah; bukan skor keamanan atau sertifikasi WCAG** |

Jumlah temuan UI: P0=0, P1=1 kelompok, P2=3, P3=0. Masalah keamanan dipisahkan dari skor UI.

## Verifikasi dan bukti

- `make check`: lulus; lint/format/typecheck web, Go vet/race, Ruff, 83 pytest, 64 tes Node, dan 5 tes skrip Python. Go integration yang memerlukan DB diuji lagi secara eksplisit dengan `TEST_DATABASE_URL` lokal.
- Go race integration auth/masterdata/attendance: lulus, masing-masing 25,7 / 8,9 / 12,3 detik.
- Dua probe Go terisolasi: lulus sebagai **reproduksi masalah**, bukan bukti masalah sudah diperbaiki. Lima perilaku berisiko teramati pada tiga kelompok temuan S1–S3.
- Tes HTTP pengenalan JPEG berulang: 1 passed; batas fake PAD dijelaskan pada S4.
- Playwright audit utama: 1 passed (20 halaman sumber dipisah menjadi 19 route + beranda pada tes tambahan); audit beranda tambahan: 1 passed. Pemeriksaan mengumpulkan temuan, sehingga status tes lulus tidak berarti semua layout lolos.
- Lima respons 401 pada request kiosk tanpa pairing sesuai state fixture; tidak ada error JavaScript yang tercatat. Health API fixture tidak membuktikan layanan AI nyata tersedia.
- Tidak menjalankan ulang build/container deployment karena tidak ada kode aplikasi yang diubah; output web yang diuji berasal dari build baseline yang sudah ada.

Raw log/probe lokal tersimpan di `.tools/security-shadcn-audit/` (ignored): `kiosk-probe.log`, `student-probe.log`, kedua source probe `.go.txt`, `replay-protocol.log`, `layout-results.json`, source audit browser, screenshot, `check.log`, `go-integration.log`, dan hasil scanner JSON. Ringkasan layout yang dapat disimpan bersama laporan: [layout-summary.json](audit-assets/2026-10-15/layout-summary.json). Tidak ada token/login nyata dalam bukti yang disalin ke docs.

Referensi pola resmi: [Sidebar](https://shadcn-vue.com/docs/components/sidebar), [Data Table](https://shadcn-vue.com/docs/components/data-table), [Dialog](https://shadcn-vue.com/docs/components/dialog), [Theming](https://shadcn-vue.com/docs/theming), [Blocks](https://shadcn-vue.com/blocks).

## Urutan perbaikan yang disarankan

1. S1 dan S2: rapatkan otorisasi scope pada backend bersama regresi database. Uji read dan write lintas kelas, bukan hanya menu/filter frontend.
2. S3: satukan pencabutan credential panel/perangkat dengan validasi operator yang masih berlaku.
3. S4: buktikan ketahanan replay/PAD pada model nyata dan tetapkan batas penggunaan absensi otomatis berdasarkan hasilnya.
4. U1: `$impeccable adapt` pada PageHeader dan kelompok tindakan bersarang; verifikasi ulang lima lebar, teks panjang, dan tombol dapat dipakai.
5. U2–U4: `$impeccable harden` untuk aksi tabel/keadaan fitur, `$impeccable adapt` untuk grafik sempit, kemudian `$impeccable polish` sebagai pemeriksaan terakhir. Perintah dapat dikerjakan satu per satu atau bersama; jalankan ulang `$impeccable audit` setelah perbaikan.

Tidak memerlukan pergantian framework, pemasangan ulang seluruh komponen, atau abstraksi baru untuk memperbaiki temuan ini. UAT sekolah, akurasi hold-out, serangan fisik PAD, beban banyak kamera, image CVE, dan konfigurasi Coolify/HTTPS tetap perlu diuji pada lingkungan target.


## Hasil remediasi — 16 Oktober 2026

Bagian ini menggantikan status temuan baseline di atas. Perbaikan kode diterapkan di branch yang sama dan layanan lokal diperbarui, tanpa reset, seed ulang, pendaftaran wajah ulang, atau perubahan catatan kehadiran nyata. Tidak dilakukan deployment Coolify.

| Temuan | Status kode dan bukti |
| --- | --- |
| S1, batas kelas | Diperbaiki. Mutasi penempatan memeriksa cakupan siswa asal dan kelas tujuan; import memakai guard kelas bersama. Pengelolaan penugasan dibatasi scope sekolah, termasuk perpanjangan/pencabutan. Regresi SQL/HTTP menolak operasi lintas kelas dan tetap menerima operasi pada kelas sendiri. |
| S2, pairing lintas kelas | Diperbaiki. Pilihan sesi difilter dan pairing memeriksa izin pada kelas sesi. Token valid untuk sesi di luar cakupan ditolak tanpa menghabiskan token. |
| S3, credential perangkat | Diperbaiki. Setiap penggunaan memeriksa pengguna/role/izin/penugasan aktif. Perubahan akun/role sensitif dan pencabutan penugasan membatalkan sesi perangkat. Expiry perangkat dengan scope kelas dibatasi akhir penugasan saat pairing; akses baru tidak menghidupkan credential lama, termasuk tanpa request selama jeda. |
| S4, replay protokol | Diperkuat, **ketahanan spoof fisik belum dibuktikan**. Izin server berlaku 10 detik, terikat sesi perangkat dan sekali pakai; fingerprint JPEG tersimpan sekitar 24 jam, tanpa foto. AI menolak piksel identik pada cache terbatas per perangkat. Gambar yang diubah/dikompresi ulang, video dan kamera virtual tetap memerlukan evaluasi. |
| Kunci sekolah saat inferensi | Diperbaiki. Inferensi di luar transaksi; credential/sesi serta revisi galeri diperiksa lagi sesudahnya. Tes inferensi tertahan membuktikan read sekolah tidak ikut tertahan; perubahan galeri saat inferensi ditolak. Revocation/expiry diuji pada jalur autentikasi bersama, bukan setiap kombinasi perlombaan inferensi. |
| Header web | CSP nonce per respons, HTML no-store, frame-ancestors none/X-Frame-Options DENY, nosniff, referrer, Permissions-Policy, HSTS. Zod memakai konfigurasi jitless resmi agar tidak memerlukan unsafe-eval. Inline style tetap dibutuhkan chart/primitive. |
| U1, tombol header | Diperbaiki pada PageHeader bersama dan grup bersarang siswa/perangkat: tombol membungkus dalam lebar layar, tetap rata kanan dan dapat dioperasikan. |
| U2, aksi peran | Memakai TableActions bersama: ikon bernama/tooltip, dengan dialog konfirmasi hapus tetap dipertahankan. |
| U3, grafik sempit | Tick tanggal dikurangi pada kontainer sempit; data dan tooltip lengkap dipertahankan. Judul, deskripsi, dan tombol laporan ditumpuk pada mobile. |
| U4, beranda | Petunjuk stabil mengikuti alur petugas dan konfirmasi kehadiran. Status koneksi awal konsisten antara server/browser sehingga tidak memicu hydration mismatch. |

Review terpisah menemukan celah revival credential saat jeda penugasan dan dokumentasi protokol lama. Keduanya diperbaiki dan diperiksa ulang tanpa temuan actionable tambahan. Regresi jeda penugasan gagal sebelum perbaikan, kemudian lulus.

### Verifikasi akhir

- `make check`: lulus lint/format/typecheck, Go vet/race, Ruff, **84 tes Python AI, 65 tes Node, 5 tes skrip Python**.
- Integrasi database dengan `-race` untuk auth/masterdata/attendance lulus; menggunakan schema sementara, bukan data sekolah nyata.
- Build web/API/bootstrap serta import runtime AI lulus. Perubahan UI terakhir dibangun ulang sesudah pemeriksaan visual.
- Migrasi 13 berhasil diterapkan pada database lokal. API ready dan halaman login lokal HTTP 200; nonce CSP dan no-store hadir. Login panel lokal berhasil. Pipeline AI ready; evaluasi hold-out/liveness/calibration tetap `not_evaluated` dan bukan sertifikasi akurasi.
- Gitleaks pada diff sumber dan file sumber baru tidak menemukan secret. Audit dependensi web terbaru tetap berisi **19 entri paket terdampak dari 3 advisory tooling**, dengan pengecualian repository berakhir 15 November 2026. Gate output runtime lulus; tidak dinyatakan bebas seluruh CVE. Hasil Go/Python scanner baseline ada pada bagian audit awal; dependency lock tidak berubah.

- Playwright akhir: **12 tes lulus**, termasuk rekam masuk/pulang fixture, laporan setelah reload, CSP, tema, dialog, aksi tabel dan regresi hydration. Audit **100 kombinasi** (20 halaman × lebar 320/390/768/1024/1440) tidak menemukan luapan halaman atau tombol tanpa nama. Lima 401 berasal dari kiosk fixture tanpa pairing; tidak ada peringatan hydration setelah perbaikan.
- Bukti sintetis sesudah perbaikan: [ringkasan layout](audit-assets/2026-10-16/layout-summary.json), [ringkasan mobile](audit-assets/2026-10-16/overview-320-after.png), [tombol detail siswa](audit-assets/2026-10-16/student-detail-320-after.png), [perangkat](audit-assets/2026-10-16/devices-320-after.png). Screenshot baseline tetap dipertahankan sebagai bukti sebelum perbaikan.

### Batas sebelum membuka staging

Uji serangan foto/video fisik, sampel hold-out terpisah, kalibrasi kamera, beban sekolah dan beberapa kamera, image OS/CVE scan, DB runtime grants, TLS/proxy, shared rate limit bila menambah replica, serta backup/restore host target tetap diperlukan. Gunakan satu replica API sampai pembatasan bersama tersedia. Data sekolah lokal tetap ada; tidak ada wajah pribadi dalam bukti UI yang disimpan.

Log akhir lokal: `.tools/security-remediation/` (ignored). Kontrak challenge/recognize diperbarui pada kedua berkas OpenAPI biometrics dan lifecycle; runbook menjelaskan urutan migrasi dan batas proteksi.
