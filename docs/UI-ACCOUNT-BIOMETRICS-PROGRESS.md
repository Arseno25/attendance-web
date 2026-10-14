# UI, akun dan pendaftaran wajah — 14 Oktober 2026


Catatan historis: modul wali/persetujuan telah dihapus dari alur aktif pada 14 Oktober 2026. Pendaftaran tanpa persetujuan berlaku di lokal dan produksi; tema kini berada di Pengaturan → Tampilan. Lihat [runbook terkini](LOCAL-RUNBOOK.md).

Pekerjaan lanjutan dijalankan di lokal sesuai arahan pengguna. Dashboard tetap mengikuti shadcn-vue New York/dashboard-01 yang sudah terpasang, menggunakan komponen registry. Pengguna mengonfirmasi belum memiliki dataset wajah uji independen; akurasi/liveness tetap **belum dievaluasi** dan kehadiran otomatis tidak diaktifkan.

## Perubahan

- Aturan Zod dipisahkan per domain di `apps/web/app/lib/validation`. Form akun, pengguna, akademik, siswa/import/penempatan, jadwal/kalender, absensi/koreksi/checkout, consent, perangkat, pairing dan filter laporan/audit memakai aturan yang sama dengan batas API. Password dihitung dalam byte dan tidak dipangkas; teks domain mengikuti Unicode code point. Pemeriksaan server tetap wajib.
- `useFormValidation` menampilkan error per field melalui shadcn Field/FieldError, menandai ARIA, memperbaiki pesan secara reaktif dan memfokuskan field tidak valid setelah submit. Handler bertahap berada di fungsi script, bukan banyak pernyataan inline di template.
- `PageHeader` mengatur judul/deskripsi dan aksi utama di kanan atas; aksi modal tetap pada DialogFooter. TablePagination memakai PaginationContent, Item, Ellipsis, Previous dan Next resmi dengan nomor halaman dan state aktif. Menu tindakan menggunakan DropdownMenu/TableActions.
- `/admin/account` tersedia dari menu akun untuk semua peran login. Nama/email dan password opsional dapat diperbarui. Perubahan email/password wajib memverifikasi password sekarang, merotasi cookie/CSRF dan mengakhiri sesi lama; nama saja mempertahankan sesi. Payload hanya memuat field yang boleh diubah. Audit tidak memuat password maupun email/nama baru.
- ModeToggle memakai shadcn DropdownMenuRadioGroup dan `@nuxtjs/color-mode`: Terang, Gelap dan Sistem, tanpa mengganti token default shadcn. Pilihan tersimpan per browser, tersedia di header panel, login dan kiosk.
- `/admin/biometrics` memberi seleksi siswa dengan metadata terbatas kepada petugas enroll/revoke, tanpa memberi akses akademik tambahan. Dialog memakai StudentFaceEnrollment yang sama dengan detail siswa. Status wajah diperbarui setelah consent berubah atau siswa diedit. Enroll dan revoke tetap memiliki izin tulis terpisah.
- FaceCamera adalah komponen reusable untuk preview, JPEG sementara dan kotak wajah. Mempertahankan aspect ratio kamera; box dinormalisasi dan dibatasi dalam frame. Kamera berhenti saat halaman tersembunyi, offline, sesi habis atau komponen ditutup. Hasil izin/network yang terlambat tidak menyalakan kamera atau menampilkan identitas lama.
- Pendaftaran memerlukan siswa aktif, consent berlaku, 3–5 JPEG berbeda, pemeriksaan satu wajah/ukuran/blur/konsistensi. YuNet dan SFace terpin menghasilkan embedding CPU yang dienkripsi AES-GCM sebelum masuk DB. Foto/embedding plaintext tidak ditulis atau dilog. Template berstatus provisional.
- `/absensi` mengirim frame dengan cookie/CSRF perangkat dalam request berurutan. Model mengembalikan deteksi nyata `no_face`, `unknown` atau `inconclusive`. Nama hanya dirender jika server menerima identitas dengan evidence evaluasi dan mengonfirmasi transaksi kehadiran. Kandidat yang ditolak tidak membocorkan nama/ID atau mencatat kehadiran.
- Migration7 menambah template ciphertext, metadata dan revision/purge. Withdrawal/revoke/status nonaktif menghapus template secara transaksional; worker menghapus template consent expired. Gallery eligibility dibaca langsung per request, tanpa cache template.

## CLI dan dokumentasi

Context7 dipakai untuk Zod, shadcn-vue/Nuxt color-mode, Vue, OpenCV dan Prettier; MCP shadcn-vue dipakai untuk registry/audit. Komponen UI yang dibutuhkan sudah dipasang dengan CLI shadcn-vue pada tahap dashboard sebelumnya; komponen reusable baru menyusun primitive tersebut.

```bash
npm --prefix apps/web install --save-exact zod@latest
cd apps/web
npx nuxt module add color-mode
cd ../..
npm --prefix apps/web install --save-exact @nuxtjs/color-mode@4.0.1
npm --prefix apps/web install --save-dev --save-exact prettier
cd services/ai
../../.tools/uv add opencv-python-headless cryptography
```

Lockfile: Zod4.6.7, color-mode4.0.1, Prettier3.9.10, OpenCV headless5.0.0.93 dan cryptography50.0.2. Tidak menambah library form/table lain. Prettier mengatur seluruh kode web authored; primitive registry tidak diformat ulang. `format:check` masuk `make check`. Seluruh test Go dengan DB dijalankan dalam `make db-test`, termasuk profile/biometric yang sebelumnya tidak cocok dengan filter nama `Database`.

Kontrak endpoint: [account.json](openapi/account.json), [biometrics.json](openapi/biometrics.json). Model, key dan protocol evaluasi ada di [README AI](../services/ai/README.md). Konfigurasi lokal/private dan batas virtualenv sementara ada di [runbook](LOCAL-RUNBOOK.md).

## Verifikasi

- Lint, Prettier check, Nuxt typecheck, Go vet/gofmt/race, Ruff, bootstrap test dan **25 test Node** berhasil. Test Node mencakup batas form, template Vue produksi, privacy label/box, ukuran JPEG, lifecycle kamera, heartbeat lama dan hasil recognition setelah kamera berhenti.
- **23 test Python** berhasil, termasuk crypto/AAD, penolakan kualitas dan smoke CPU model nyata dengan input sintetis. Ini bukan pengukuran akurasi wajah.
- Test PostgreSQL memakai schema disposable: akun/rotasi/scope/race logout, consent/enroll/revoke/cross-school, unknown tanpa identitas/kehadiran, gallery revision serta workflow akademik/absensi. Test tidak mengubah akun/wajah nyata.
- Build produksi Nuxt dan binary Go berhasil; Compose config valid; migration7 diterapkan. Runtime lokal diperbarui di web3001/API8080/AI8001. AI readiness menunjukkan pipeline siap, `recognition_enabled:false` dan `MODEL_LIVENESS_NOT_EVALUATED`.
- Browser memverifikasi login invalid, menu akun, form invalid/fokus/error reaktif, tema gelap/terang/sistem dan persistence setelah reload, pagination audit halaman1→2, daftar/dialog wajah dan form akun desktop/mobile. Tidak ada overflow halaman pada390px; tabel memakai scroll internal. Tidak mengubah password/akun atau membuat consent/wajah melalui browser.
- Review terpisah menemukan dan memperbaiki akses operator enrollment, akses revoke-only serta status stale setelah deactivation. Status template juga diuji agar tetap konsisten ketika ada consent berlaku yang lebih baru. Review konfirmasi tidak menemukan masalah lain yang dapat direproduksi. Detector desain mekanis tidak menemukan issue; pemeriksaan browser tetap menjadi bukti visual.

## Belum dinilai

Kamera fisik/izin capture, pendaftaran manusia nyata, akurasi identitas, spoof/liveness, kalibrasi ambang/margin, latency hardware dan G3 restore template belum diuji. Alur evaluator trace anonim tersedia; gunakan sampel berizin yang terpisah dari pendaftaran. Tidak ada angka akurasi atau persetujuan otomatis dari smoke test/instalasi. Coolify dan gate produksi mengikuti rencana induk dan belum dijalankan.

Disk home hampir penuh; hanya virtualenv project dipindahkan ke `/tmp` dan disymlink. Virtualenv development perlu dipulihkan setelah reboot; DB, model dan kunci tetap pada storage asal. Image AI sudah dibangun dari Dockerfile/lockfile dan diuji dengan model CPU, lalu dijalankan memakai additional image store di `/var/tmp/attendance-ai-build/storage` karena import ke storage home kehabisan ruang. Runtime AI tidak bergantung pada virtualenv host. Pertahankan storage tambahan dan gunakan command restart pada runbook; deployment di host cukup ruang memakai image/Compose standar.


## Pembaruan aturan password — 14 Oktober 2026

Sesuai aturan terakhir pengguna, password baru minimal 8 karakter dan harus memiliki huruf besar, huruf kecil, angka 0–9, serta simbol. Tidak ada batas maksimal password atau trimming. Form akun dan pembuatan pengguna memakai schema Zod dan komponen `PasswordRequirements` yang sama; indikator menandai aturan yang terpenuhi tanpa menampilkan password. Konfirmasi harus persis sama dan perubahan sensitif tetap memerlukan password saat ini.

Go memakai aturan komposisi yang sama pada hashing/creation/bootstrap/profile. Login dan verifikasi password existing tidak memaksakan aturan baru, sehingga akun lama tetap dapat digunakan. Batas request JSON 8 KiB, limiter dan concurrency hashing tetap berlaku sebagai pengaman transport; tidak ada lagi cap 128 pada password maupun input HTML.

`make check`, `make db-test` dan build Nuxt/Go berhasil. Tes mencakup batas 7/8 karakter, setiap syarat komposisi, Unicode, simbol dibanding whitespace, password di atas 128 karakter, login setelah perubahan password panjang, preservasi spasi, konfirmasi, rotasi sesi dan rollback. Fixture bootstrap yang memerlukan password baru diperbarui ke policy ini. Akun/password nyata pengguna tidak diganti selama pengujian; hanya schema disposable untuk perubahan password.


## Notifikasi Sonner — 14 Oktober 2026

Sonner shadcn-vue dipasang melalui CLI resmi `npx --yes shadcn-vue@latest add sonner --yes` dari `apps/web`. Paket `vue-sonner` dipin tepat 2.0.9; dependency Lucide tetap exact 1.56.0. Font Google duplikat yang disisipkan CLI dihapus karena Inter lokal sudah tersedia. Toaster registry, stylesheet resmi, dan satu host `ClientOnly` di `app.vue` dipakai untuk semua route. Host mengikuti tema terang/gelap/sistem, rich colors per jenis status, posisi kanan atas, durasi enam detik, dan tombol tutup dengan label Bahasa Indonesia.

Semua state/panel sukses sementara dipindahkan ke `toast.success`; kegagalan tindakan memakai `toast.error(apiMessage(error))`. Sudah tercatat memakai `toast.info`; preview CSV tidak valid memakai `toast.warning` sambil mempertahankan detail per baris. Login/logout, pairing, CRUD akademik/pengguna/siswa/perangkat, jadwal/kalender, attendance/corrections, export, consent dan enrollment memakai host yang sama. Tidak memasukkan token pairing, password atau gambar wajah dalam toast.

FieldError, error loading yang memerlukan retry, empty states, streaming camera quality/error, serta warning evaluasi/consent tetap di konteksnya. Tidak ada toast berulang dari polling kamera/heartbeat. Tidak ada perubahan API, data, auth, password policy, atau syarat keberhasilan operasi.

Context7 memakai dokumentasi Sonner shadcn-vue dan Vue Sonner; MCP shadcn-vue dipakai untuk command/audit. `make check` berhasil dengan 33 tes Node dan 64 pytest serta lint/format/typecheck/Go/Ruff. Tiga tes baru menjalankan handler akun production melalui compiler Vue dan observer Sonner nyata: success hanya setelah operasi selesai, submit ganda ditolak, error tunggal tanpa success dan form dipertahankan, serta validasi field tidak mengirim request/toast. Build production Nuxt berhasil.


Browser runtime memverifikasi satu host Sonner, success setelah save profil dengan nilai yang sama, tema sistem/gelap dan terang, close button, dan ukuran390px tanpa overflow. Tidak mengisi field password atau mengganti identitas/kredensial. Tema/viewport dikembalikan setelah pengujian. Bukti `.tools/sonner/success-desktop.png` dan `success-mobile-light.png`. Detector mekanis target UI mengembalikan tanpa temuan.
