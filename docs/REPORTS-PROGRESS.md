# WP-05 — Ringkasan kehadiran, laporan dan ekspor

Base 3c8641d; plan 07-IMPLEMENTATION-PLAN.md WP-05 / UI-04 disetujui pengguna.

- [x] Kontrak report/summary dan uji gagal sebelum endpoint.
- [x] Query roster-scoped, summary tanpa double count, trend tujuh hari dan recent records.
- [x] Export penuh filtered + permission/revocation + audit atomik + CSV dengan mitigasi injection awal.
- [x] Dashboard tanggal/kelas dan reports tahun/kelas/siswa/periode/status/search/pagination/export, audit filter.
- [x] Integration, check/build, browser desktop/mobile/download, review dan commit.

Ruling: satu baris report adalah satu siswa pada satu roster sesi non-cancelled; placement saat ini tidak mengganti histori. not_recorded berasal LEFT JOIN; absent hanya record disahkan engine WP-04. Total roster adalah peluang kehadiran, siswa unik ditampilkan terpisah. attended=present+late, late subset; rate denominator seluruh roster hasil filter.
Ruling: default tanggal memakai timezone sekolah; rentang maksimal 366 hari. CSV memakai query/filter/otorisasi yang sama, tanpa pagination, maksimal 10.000 baris, gagal jelas tanpa partial download jika melewati cap. GET format=csv memerlukan reports.export, audit commit sebelum byte CSV; tidak mengubah business state. Kontrol export hanya admin/super/guru sesuai permission existing; operator ditolak.
Ruling: semua read/export mengecek sesi dan class assignments live di bawah lock sekolah shared; writes/permission revoke menunggu lock yang sama. Guru hanya membaca kelas assignment aktif saat ini, termasuk roster historis kelas itu.
Ruling: statistik dan trend tujuh hari memakai Table/Card shadcn Vega terpasang; tidak menambah dependency chart dekoratif pada P0 (plan §7). Semua statistik dapat dibaca tanpa graph, dan CSV/report lengkap. Kiosk/AI/WebSocket bukan WP-05.


## Verifikasi akhir — 13 Oktober 2026

- TDD awal GET reports gagal 404 sebelum handlers; integration kemudian lulus. Date/status/year/class/student/session/search memakai satu query roster; pagination hanya memotong data, bukan totals atau CSV. Cancelled tidak masuk, bukan semua siswa master.
- `make db-test` exit 0 dengan race detector untuk auth/masterdata/attendance pada schema terisolasi yang dibersihkan. Kasus meliputi not_recorded awal, late tidak double count (1/3=33,3%), tujuh tanggal trend, status/per-student filter, historical placement, teacher other class, revoked assignment (CSV header only), operator denied, invalid date/UUID/range, CSV seluruh hasil tanpa pagination, export audit sebelum output, audit actor/action/local date filter, dan cap dari 10.385 roster fixture: 400 tanpa partial file atau audit sukses palsu.
- Sesi fixture dipakai ulang bila masih valid; grant/session revocation tetap memaksa login baru. Batas login produksi tidak diubah. Tanggal besok fixture diturunkan dari tanggal sekolah, tidak timezone host.
- Review fresh menemukan dirty form tidak dinormalisasi. Browser mereproduksi ekspor disabled setelah `  Siswa  ` diterapkan; form kini menyalin filter canonical server, browser membuktikan nilai `Siswa` dan ekspor enabled.
- CSV quote setiap cell (termasuk semicolon), double quote escaped, formula-like prefixes ASCII/full-width/control/whitespace diberi apostrophe. Uji semicolon gagal sebelum quote-all, lalu lulus; CSV reader membuktikan kolom/rows/formula prefix. Panduan primer [OWASP CSV Injection](https://community.owasp.org/attacks/CSV_Injection): mitigasi untuk import awal; spreadsheet dapat menghapus escaping ketika disimpan ulang, bukan jaminan untuk setiap aplikasi/resave.
- CSV memakai BOM UTF-8 dan CRLF rows; waktu hadir lokal memakai timezone snapshot sesi. Go stdlib time/tzdata embedded agar Alpine tidak bergantung paket OS; timezone dimuat sekali per zone per ekspor. Metadata audit menyimpan filter/row count dan search_filtered, tanpa teks pencarian mentah atau isi CSV.
- `make check`, Nuxt production build, Go build/vet, secret scan redacted, diff-check, detector desain terarah exit 0. Runtime API/web diperbarui; readiness 200. Tidak menambah dependency atau migration; indeks/FK dari WP-03/04 dipakai.
- Browser 1440x900 dan 390x844: laporan/filter canonical, download nyata, dashboard tujuh hari/recent NIS masked, menu mobile, audit action filter dan actor name, satu main, no page overflow, console warn/error kosong. Viewport di-reset. CSV demo berhasil di Downloads, BOM efbbbf, dua baris (header+roster), row DEMO-001; semua data dummy lokal.
- Screenshot lokal ignored: `.tools/reports-desktop.png`, `.tools/reports-dashboard.png`, `.tools/reports-mobile.png`. Logs `.tools/reports-final-db.log`, `.tools/reports-complete-check.log`, `.tools/reports-final-build.log`. Kontrak: [reports.json](openapi/reports.json), termasuk audit filters/actor_name. Context7 shadcn-vue Table composition dan ofetch response/error parsing dipakai; komponen terpasang digunakan ulang.

Batas: range 366 hari dan CSV 10.000 row; untuk ekspor lebih besar pecah rentang/filter, tidak ada truncation diam-diam. Chart dekoratif, PDF/XLSX dan realtime bukan P0 paket ini. CI remote dan deployment produksi belum dijalankan; temuan dependency upstream sebelumnya belum ditangani. WP-06 memerlukan kamera/dataset uji berizin dan evaluasi model/liveness sebelum integrasi biometrik.
