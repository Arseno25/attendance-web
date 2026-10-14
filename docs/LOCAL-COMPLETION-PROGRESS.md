# Kelanjutan lokal — 13 Oktober 2026


Catatan historis: modul wali/persetujuan telah dihapus dari alur aktif pada 14 Oktober 2026. Pendaftaran tanpa persetujuan berlaku di lokal dan produksi; tema kini berada di Pengaturan → Tampilan. Lihat [runbook terkini](LOCAL-RUNBOOK.md).

Plan: `07-IMPLEMENTATION-PLAN.md`, base `8ea8321`. Pengguna meminta meneruskan seluruh plan, kamera tersedia, dataset akan didaftarkan, model dipasang oleh agent; Coolify ditunda atas instruksi pengguna dan semua pekerjaan fokus lokal.

## Status dan dependency

| Paket | Hasil lokal | Belum dapat dinyatakan selesai |
|---|---|---|
| WP-01–05 / G1 | Core auth, akademik, manual, koreksi, reports sudah tersedia | Remote CI dan UAT sekolah |
| WP-06 / G2 | YuNet + SFace terunduh, SHA/ukuran dari pointer resmi, ONNX CPU load; installer reproducible, evaluator hold-out FPIR/FNIR/attack/latency | Kamera nyata, dataset berizin, liveness, calibration, exact matching dan stability end-to-end |
| WP-07 / G3 | Wali terverifikasi, consent dengan expiry/policy/evidence reference, withdrawal atomik + immutable tombstone, UI histori | Ciphertext enrollment, gallery invalidation/purge/restore menunggu G2; enrollment endpoint 503, tidak menerima embedding/foto |
| WP-08 | Registrasi/revoke perangkat, one-time pairing 5 menit, cookie HttpOnly 10 menit, live session check/heartbeat, halaman `/absensi` mandiri dengan preview kamera lokal | Capture/upload protocol produksi dan renewal otomatis menunggu hasil WP-06 |
| WP-09 | Endpoint recognition device-authenticated, CSRF+origin, fail-closed 503 | Transaksi automatic attendance belum diaktifkan; memerlukan G2 dan G3 |
| WP-10 | Import preview/hash/expiry/revalidation/atomic commit, checkout idempotent, WebSocket invalidation + REST refetch/reconnect | UAT data sekolah |
| WP-11 | Check/build/DB tests, local preview, runbook dan rehearsals | Produksi, key management biometrik, load/performance aktual dan UAT |
| WP-12 | Urutan rollout manual → shadow → automatic tetap berlaku | Pilot/sign-off/monitoring nyata; tidak dapat direkayasa dari fixture |

## Keputusan pelaksanaan

Ruling: pekerjaan independen dari G2 diteruskan, tetapi G2/G3 tidak dianggap lulus hanya karena model bisa dimuat. Biaya bila salah: enrollment produksi harus tetap tertutup sampai evaluasi nyata. Ini mengikuti plan §8/10 dan dependency WP-07/09, bukan persetujuan tambahan yang dibuat agent.

Ruling: YuNet 2023mar dan SFace 2021dec FP32 adalah baseline terpin OpenCV Zoo commit `47534e27c9851bb1128ccc0102f1145e27f23f98`, lisensi MIT/Apache-2.0 tersedia di subdirektori masing-masing. Fixed-shape baseline dipilih untuk CPU ONNX yang terpasang; bukan klaim model paling akurat atau model/liveness yang layak sekolah. Biaya bila salah: evaluasi dapat mengganti model dan template sebelum enrollment dibuka.

Ruling: wali/consent menyimpan referensi bukti privat, tanpa upload scan identitas atau bukti ke log/audit. Officer menggunakan `consents.manage`; pengelolaan guardian/consent ditolak untuk teacher/operator. Guardian ID harus milik siswa dan sekolah sama melalui FK. Consent hanya untuk siswa aktif, expiry mandatory, tidak boleh menggandakan consent yang belum expired/withdrawn. Withdrawal idempotent dan tombstone append-only; belum ada template/cache yang perlu purge pada runtime ini.

Ruling: pairing token 256-bit acak disimpan hashed, satu token per device, lifetime 5 menit; sesi kiosk 10 menit di cookie HttpOnly/SameSite Lax/Secure pada HTTPS. Pairing membutuhkan user login dengan devices.health. Revoke membatalkan token dan semua device sessions secara atomik. Renewal otomatis belum dibuat sebelum capture protocol terukur; biaya: operator memasangkan ulang tiap 10 menit. Browser tidak menyimpan token, foto atau frame di storage/cache. Preview stop pada hidden/offline/expiry/leave dan track-ended, pending getUserMedia juga dibatalkan secara logis dan tracks yang terlambat datang dihentikan.

Ruling: import satu kelas per batch, max 500 rows / 128 KiB JSON (UI max 120 KiB file), header UTF-8 `nis,nisn,full_name`. Immutable canonical payload/hash disimpan server, expires 15 menit. Commit hanya creator dengan izin live; validasi ulang kelas dan uniqueness dalam satu transaksi, retry hasil committed sama. Payload dihapus segera saat commit dan saat expired melalui lifecycle worker (interval 1 menit). Metadata batch/row count tetap tersedia untuk audit, nama/CSV tidak masuk audit.

Ruling: checkout manual hanya hadir/terlambat, setelah check-in, sesi open dalam window, tanpa correction pending; server time, reason mandatory, event ledger/fingerprint scoped actor/operation, first checkout preserved, events/audit atomic. Approved correction membatalkan checkout summary agar waktu pulang lama tidak melekat pada status/check-in yang dikoreksi; event historis tetap ada. Biaya: petugas perlu mencatat pulang ulang bila correction mengubah catatan sebelum akhir sesi.

Ruling: WebSocket single-session invalidation only, origin wajib exact, live auth/class scope checked setiap 2 detik; disconnected pada session/permission revoke. Client refetch penuh saat reconnect, REST tetap sumber kebenaran. Bounded 32 connections/instance, 30 menit lifetime, 256-byte incoming, ping/pong deadlines. Query revision scoped DB setelah commit, tidak mengirim nama/NIS/row melalui socket. Biaya: notifikasi maksimal lag sekitar 2 detik, query polling; naikkan menjadi outbox/broker hanya bila beban terukur membutuhkan.

## Bukti

Log ignored `.tools`: model-red, models-installed.json, continue-red/green, device-green, import-red/green, checkout-red/green, ws-red/green, local-final-check. Integration memakai Postgres nyata pada schema fixture terisolasi, tidak mengubah data siswa nyata. Kontrak dan bukti browser/build diperbarui di akhir paket.

## Review dan perbaikan akhir

Fresh reviewer pertama tidak dapat berjalan karena usage limit; pengganti fresh reviewer tersedia menyelesaikan review read-only. Empat Important dan tiga Minor dilaporkan. Tiga Minor diregrade Important karena label pairing adalah verifikasi scope perangkat, late mount membuat heartbeat tetap berjalan setelah keluar, dan kehilangan notifikasi rekonsiliasi membuat state operasional salah. Tidak ada deferred minors.

Final: fixed operator discovery/filter — `/kiosk/session-options` hanya devices.health, paginated server open+clock-eligible metadata; regression404→200 dan UI operator0choices→1choice.
Final: fixed resumed kiosk re-pair — watch unpaired state loads operator+options, mounted response disposed guard; Vue regression expired0choices→1choice dan late timers2→0.
Final: fixed pairing labels — response kini device_name/session_name dari transaksi yang sama; DB integration asserts labels.
Final: fixed live editor invalidation — pending flag tetap queued selama form diblokir; watch flush setelah dismissal; regression callback premature1→0 lalu dismissed1.
Final: fixed reconciliation-only invalidation — revision includes reconciled_at; real WebSocket test timeoutRED→messageGREEN setelah semua roster tercatat dan reconcile tanpa record baru.

Ruling: evaluasi model/liveness/camera aktual dan ciphertext/gallery/purge/restore sengaja belum dinilai reviewer sebagai fitur aktif; gateG2/G3 tetap belum lulus, biaya bila salah adalah automatic attendance tetap tertutup. Concurrent consent/import/checkout mendapat shared school write lock; regression memakai PostgreSQL/race. Broker dan renewal otomatis belum diperlukan untuk lingkup lokal; limit dan biaya sudah dicatat di atas. Migration down setelah checkout tidak digunakan; forward migration dan restore core ke DB disposable dipilih, biaya bila rollback diperlukan adalah penilaian schema compatibility manual.

Final: fixed operator account isolation after pairing — schema-fixture auth/me remained200RED and Vue identity remained non-nullRED; pairing now revokes that user session inside the same transaction, clears user cookie after commit and clears frontend identity while device cookie remains active. This prevents a public kiosk retaining access to the private panel. Re-pairing requires operator login again. Token and user-cookie contents never enter audit; handoff audit only boolean operator_session_revoked.

## Bukti akhir lokal

- Forward migration5/6 berhasil pada DB lokal; schema6. `make db-test` dengan race detector lulus auth/master/attendance, termasuk default operator tanpa attendance grant, pairing single-use+labels+user-session handoff401, device revoke/CSRF, no sensitive audit metadata, consent/withdrawal, import hash/retry/invalid batch rollback, checkout first time+idempotency, real WebSocket origin/revocation dan reconciliation-only notification. Log `.tools/kiosk-isolation-green.log`.
- `make check`: lint/typecheck, Go vet/gofmt/race, Python ruff+8tests, bootstrap unittest,5Vue regression flows. UI regression sebelumnya gagal untuk discovery/resume/late timers/editor queue/operator identity, lalu lulus. Full build Go/Nuxt/ONNX CPU lulus; runtime web/API diperbarui, image API menyertakan migrations6; AI container memuat model dari volume readonly dan manifest pinned.
- Model install load+SHA pada host dan AI container berhasil. Synthetic zero-tensor smoke menghasilkan output finite, YuNet detector outputs dan SFace128dimensions. Latency16.30/17.93ms dari satu synthetic run bukan end-to-endP95 atau akurasi hold-out. Artefak `.tools/model-smoke-evidence.json`, `.tools/models-installed.json`, `.tools/local-ai-models.log`. Warning initializer SFace upstream menyangkut optimization, tidak diubah/re-export sehingga checksum tetap identik.
- Backup GnuPG AES256 encrypted pipe→integrity-verified decrypt→DB disposable restore berhasil7.89sec; counts students1/attendance1/tombstones0/schema6 sama. DB/key/backup latihan dihapus; source DB tidak ditimpa. Tombstone count0 bukan bukti restore lifecycle biometrik; G3 tetap pending.
- Browser: login akun development, menus perangkat/import, Sheet labels, private consent detail, kiosk separate layout/open session choices. Desktop1440 dan mobile391: no document overflow; CardAction consent dipindah row3 mobile (description width295px), horizontal table scroll tetap internal. Kamera tidak diakses oleh agent, foto tidak diunggah. Device pairing/CSV commit/withdrawal dibuktikan integration+Vue tests, bukan penggunaan wajah/data siswa nyata. Screenshot `.tools/local-devices-desktop.png`, `.tools/local-consent-desktop.png`, `.tools/local-consent-mobile.png`, `.tools/local-kiosk-desktop.png`, `.tools/local-kiosk-mobile.png`. Console panel warn/error kosong pada pemeriksaan; viewport di-reset, kiosk disediakan sebagai tab hasil.
- `make compose-check` dan diff-check lulus. Secret scan redacted staged diff tanpa temuan. Go govulncheck0reachable/0imported vulnerabilities;1module advisoryGO-2026-5932 pada openpgp x/crypto tidak diimpor/dipanggil (Argon2id dipakai), fixedN/A. npm audit tetap19 (14high/5moderate/0critical); tidak dinyatakan bersih dan tidak force downgrade/major fix. Gate release produksi belum lulus.
- Context7: ONNX CPU InferenceSession, shadcn-vue accessible Field/Sheet/Dialog dan Gorilla WebSocket deadlines/origin/concurrency. CLI resmi `go get github.com/gorilla/websocket@latest` menghasilkan pinv1.5.3, `go mod tidy`; model installer memakai official pinned OpenCVZooURLs. Tidak menambah dependency kamera/encryption/liveness secara spekulatif sebelum protocol dan evaluation tersedia.

Ruling: versi source tetap di branch `foundation` sebagai pekerjaan lokal; tidak push/merge/deploy Coolify karena arahan lokal. Biaya bila integrasi berikutnya diperlukan: reviewer/UAT dan keputusan release sebelum merge/deploy. Semua paket yang memerlukan data/hardware/governance nyata tetap pending pada tabel status; tidak ada klaim keseluruhan plan atau biometrik selesai.
