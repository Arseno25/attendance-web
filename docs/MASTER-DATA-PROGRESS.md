# WP-03 — Data akademik/siswa dan koreksi style

Base `fb78921`; rencana disetujui: 07-IMPLEMENTATION-PLAN.md DB-02/UI-02/WP-03.

- [x] Samakan block dan primitives ke preset resmi Vega lewat CLI resmi; periksa computed spacing.
- [x] Kontrak API + migration years/classes/students/placements/assignments.
- [x] Uji PostgreSQL nyata: duplikat NIS, no-overlap, transfer, tenant/class isolation, CSRF, role/revocation.
- [x] Auth scopes, endpoints master data dan audit atomik.
- [x] UI tahun ajaran, kelas, guru, daftar/detail/edit siswa; ringkasan memakai total nyata.
- [x] Checks/build/runtime, review independen dan commit.

Ruling: block dashboard-01 berasal dari new-york-v4, tetapi primitives sebelumnya Nova; CLI terkini otomatis memigrasikan style lama; semua primitives dipasang ulang melalui `shadcn-vue apply --preset vega --yes` (reka-vega). CSS warna neutral dan font Inter disajikan lokal dan tetap konsisten, tanpa varian preset campuran.
Ruling: interval penempatan/guru memakai UTC timestamptz [joined_at,left_at), batas tahun akademik dihitung dengan timezone tahun yang disalin dari sekolah. Transfer atomik memotong akhir penempatan lama dan menambah baris baru; backdating yang menimpa histori ditolak.
Ruling: class code unik per tahun; tahun tidak overlap dan hanya satu aktif. Tanggal tahun dikunci ketika sudah memiliki kelas. Tidak ada hard delete.
Ruling: administrator/super admin menulis data master. Guru hanya membaca kelas dengan assignment aktif saat ini; detail histori siswa difilter kelas yang boleh dibaca. Revocation/status/role dibaca ulang dengan lock sekolah+actor di transaksi agar akses tidak memakai snapshot sesi lama.
Ruling: operator tidak mendapat akses data master. Import/consent/biometrik/absensi belum masuk WP-03; kiosk tetap di luar panel pada tahapnya.


## Verifikasi akhir 2026-10-13

- TDD awal gagal 404 sebelum handlers dibuat. PostgreSQL integration kemudian lulus untuk auth + master data dengan `make db-test` (race detector), pada schema terisolasi yang dibersihkan sesudah tes.
- Duplikat NIS/kode kelas, overlapping years/placements/assignments, periode immutable, transfer atomik, backdating/future ditolak, reaktivasi/penempatan ulang, cross-school composite FK/404, rollback data parsial dan aktivasi tahun, batas guru/revocation, CSRF/pagination dan audit diuji.
- Review independen menemukan respons create memakai waktu awal transaksi sehingga kelas/current assignment belum terlihat. Regression test gagal dengan class_id null; seleksi temporal diperbaiki memakai statement_timestamp dan test lulus. Race respons tab diperbaiki dengan request sequence + captured tab. Catatan kontrak ditangani dengan schema responses/filter/pagination/header yang dapat dibaca mesin.
- `make check`, `make build`, `make db-test`, `git diff --check`, scanner secret (redacted) dan detector desain terarah exit 0. Build Nuxt produksi dan binary Go berjalan di container preview lokal; readiness API 200. Ini verifikasi lokal; CI remote belum dijalankan.
- Browser 1440×900 dan 390×844: login, tambah tahun ajaran/kelas/siswa demo, detail/histori, edit siswa, ringkasan total nyata, navigasi mobile, satu landmark main, font lokal Inter dan overflow halaman diperiksa. Console warn/error kosong. Screenshot `.tools/master-dashboard.png`, `.tools/master-student-desktop.png`, `.tools/master-student-mobile.png` (ignored lokal).
- Data demo jelas berlabel Demo, satu tahun aktif, satu kelas dan satu siswa pada database pengembangan. Tidak ada data pribadi asli atau password baru yang ditampilkan.
- Kontrak API: [master-data.json](openapi/master-data.json). Schema + definisi request/response/filter mengikuti handlers; migration 00003 diterapkan lokal sebelum API baru.
- CLI official shadcn-vue 2.8.2: add komponen, apply preset Vega. Dokumentasi diperoleh lewat Context7 untuk CLI/dashboard dan PostgreSQL btree_gist/range/composite FK. Font lokal mengikuti [Fontsource Inter](https://fontsource.org/fonts/inter/use).

Batas: WP-03 tidak mencakup import massal, consent, biometrik, jadwal/kehadiran atau kiosk. UI penugasan menggunakan mulai sekarang hingga akhir tahun; API mendukung periode eksplisit. Npm audit tetap 19 temuan upstream (14 high/5 moderate/0 critical); tidak menjalankan upgrade paksa yang mengubah stack di luar scope.
