# WP-04 — Jadwal, kalender dan absensi manual

Base e72baf4; pelaksanaan plan 07-IMPLEMENTATION-PLAN.md WP-04 / DB-03/04 / UI-03 yang disetujui pengguna.

- [x] Kontrak, migration dan failing integration test.
- [x] Kalender/jadwal/sesi UTC dan snapshot roster.
- [x] Manual idempotent, izin/sakit, close/reconcile dan koreksi dengan approver berbeda.
- [x] Halaman shadcn jadwal/kalender, sesi/roster/manual dan antrean koreksi.
- [x] Tests race/tenant/class/CSRF/retry, checks/build, browser desktop/mobile, review dan commit.

Ruling: jadwal per kelas, jam lokal pada timezone sekolah yang disalin; periode dalam satu hari (start < cutoff+grace < close), weekday ISO 1–7. Kalender override hari efektif/libur. Sesi menyimpan waktu dan jadwal snapshot; edit jadwal/kalender hanya memengaruhi sesi berikutnya. Libur tidak membatalkan sesi existing diam-diam.
Ruling: roster dibekukan ketika sesi dibuka berdasarkan placement efektif pada waktu mulai sesi. Status awal not_recorded, absent hanya dibuat oleh reconcile eksplisit setelah closed dan tidak ada koreksi pending; izin/sakit tidak ditimpa. Kelas/siswa yang pindah setelah pembukaan tidak menulis ulang roster.
Ruling: admin/super admin kelola jadwal/kalender. Operator dengan grant manual dapat membaca roster/mencatat dan membuka/menutup sesi. Guru membaca dan meminta koreksi hanya di kelas assignment aktif saat ini. Reconcile/approve memerlukan attendance.approve; self-approval selalu ditolak.
Ruling: idempotency UUID wajib untuk manual, scoped school/actor/operation, payload hash canonical; retry identik mengembalikan hasil tersimpan, payload/operation berbeda 409. Check-in ulang dengan UUID berbeda mempertahankan waktu pertama dan tidak menambah event. Koreksi memakai versi record sebelumnya; perubahan bersamaan menyebabkan konflik, bukan overwrite.
Ruling: create/open/close tersedia eksplisit; worker tiap menit membuat sesi hari ini dan membuka/menutup sesuai window, tanpa otomatis mengesahkan absent. Worker satu sekolah memakai lock sekolah yang sama dengan master data/auth. Tidak ada hard delete history. Jadwal, sesi, roster dan record memakai composite FK school.
Ruling: runtime/context sama dengan panel Vega WP-03; reuse komponen installed tanpa dependency baru. /absensi kiosk tetap WP-08. Settings timezone umum, report/export, checkout, AI dan pairing bukan WP-04.


## Hasil verifikasi — 13 Oktober 2026

- TDD awal: GET schedules 404 sebelum implementasi; workflow PostgreSQL kemudian lulus. Test menggunakan schema terisolasi, dibersihkan setelah run.
- `make db-test` exit 0: auth, masterdata dan attendance dengan race detector. Menguji duplicate jadwal/sesi, kalender libur, snapshot awal dan late roster addition, no premature absent, manual UUID retry/payload mismatch, dua first check-in concurrent dengan satu event, penugasan/grant revocation, teacher/cross-school isolation, CSRF, pending corrections, distinct approver, dua keputusan concurrent, stale record version, reconcile menjaga sick/excused, immutable events dan worker retry/lifecycle.
- Review fresh menemukan audit worker creation belum ditulis dan filter pending_review ditolak; regression tests gagal dahulu lalu lulus setelah perbaikan. Worker creation audit atomik dan satu kali walau retry; filter status membaca enum record lengkap tanpa memperluas status koreksi yang boleh ditulis.
- Kontrak OpenAPI membedakan input jam HH:mm dari response HH:mm:ss. UI kalender menjelaskan snapshot mencakup sesi terjadwal maupun terbuka; kalender tidak membatalkan histori diam-diam.
- Preflight PUT kalender semula tidak terdaftar di CORS allow-methods. Test kalender credentialed preflight gagal dahulu; handler CORS shared diperbaiki, tanpa memperluas origin/header allowlist atau melewati CSRF.
- `make check`, `make build`, `git diff --check` dan detector desain exit 0. Nuxt build produksi, Go binary, readiness API dan preview web berhasil. Tidak ada dependency framework/komponen baru; primitives Vega yang sudah dipasang CLI dipakai ulang. Context7 shadcn-vue mengonfirmasi komposisi accessible Sheet/Dialog dengan title/description.
- Browser desktop 1440x900 dan mobile 390x844: login, form jadwal, jadwal demo dibuat, worker membuka sesi + snapshot satu siswa, hadir dicatat via browser, form koreksi dengan waktu lokal dan ukuran mobile, kalender PUT berhasil, filter Perlu review menghasilkan empty state (bukan 400), satu main, tanpa overflow halaman dan console warn/error kosong. Ukuran viewport dikembalikan. Satu jadwal dan record berlabel Demo pada DB pengembangan; tidak ada PII asli.
- Bukti lokal ignored: `.tools/attendance-desktop.png`, `.tools/attendance-mobile.png`, `.tools/attendance-final.png`; logs `attendance-complete-{db,check,build}.log`. Detail contract: [attendance.json](openapi/attendance.json).

Batas: rekonsiliasi memerlukan sesi closed, waktu close terjadwal terlewati dan tidak ada koreksi pending; ditinjau administratif, tidak otomatis oleh worker. Periode satu hari, satu jadwal aktif/kelas; checkout dan WebSocket P1 belum dibuat. CI remote, deployment produksi dan perangkat kamera belum diuji. Audit npm upstream sebelumnya tetap 19 temuan (14 high, 5 moderate); tidak menambah dependency atau menjalankan upgrade paksa. Tahap produk berikutnya WP-05: ringkasan kehadiran, laporan/filter dan ekspor CSV dengan scope/audit.
