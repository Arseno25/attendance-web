# Login-04 dan pendaftaran kamera — 14 Oktober 2026

Implementasi lokal selesai untuk login dan alur pendaftaran kamera. Pengenalan 1:N tersedia sebagai engine internal reusable serta alat evaluasi; endpoint absensi tetap menolak identifikasi otomatis sampai bukti kalibrasi dan liveness tersedia. Tidak ada dataset wajah independen yang diberikan, sehingga akurasi tetap **belum dievaluasi**.

## UI dan komponen

- Login mengikuti block shadcn-vue `login-04`: Card dua kolom pada desktop, form satu kolom pada mobile, Field/Input/Button native, FieldError dari validasi Zod, notice sesi berakhir, dan menu tema yang sudah ada. Gambar dekoratif lokal menggantikan placeholder block; akun tetap diterbitkan sekolah.
- Modal daftar wajah menampilkan nama siswa, preview kamera, panduan posisi, kotak kualitas, progres native shadcn, lima slot frame sementara dan kontrol ambil/ulang/simpan. Minimal tiga frame berbeda; kamera dan simpan memerlukan siswa aktif serta persetujuan berlaku. SFace memeriksa konsistensi saat enrollment; preview memakai deteksi/quality YuNet dan tidak mengekstrak embedding.
- Capture menyimpan frame yang benar-benar telah diperiksa oleh model, bukan mengambil frame baru setelah hasil pemeriksaan. Request lama dibatalkan saat berhenti/menutup/berganti siswa. Reset setelah lima frame menunggu pembaruan props Vue sebelum melanjutkan preview. Kamera mengikuti lifecycle hidden/offline/leave yang sudah ada.
- Primitif Progress berasal dari CLI, tanpa mengganti komponen native existing. ESLint void-element diselaraskan dengan Prettier pada komponen authored; file registry mempertahankan format upstream.

Dokumentasi diambil melalui Context7 (`/websites/shadcn-vue`, `/unovue/shadcn-vue`, `/websites/vuejs`, `/websites/opencv_5_0`, `/vuejs/eslint-plugin-vue`) dan MCP shadcn-vue registry/audit. Referensi resmi: [blocks](https://www.shadcn-vue.com/blocks), [CLI](https://www.shadcn-vue.com/docs/cli), [nextTick](https://vuejs.org/api/general.html#nexttick), [html-self-closing](https://eslint.vuejs.org/rules/html-self-closing.html).

Command resmi dijalankan dari `apps/web`:

```bash
npm_config_cache=/tmp/attendance-npm-cache npx --yes shadcn-vue@latest add login-04 progress --yes
```

CLI membuat form block, root `pages/login/index.vue`, dan Progress, lalu melewati 27 file komponen existing tanpa overwrite. Project memakai Nuxt 4 `app/pages`; markup block diintegrasikan ke route existing `app/pages/login.vue` dan root page yang tidak dipakai dihapus. Import font Google duplikat dihapus karena Inter lokal sudah terpasang. Dependency existing yang diubah CLI dikembalikan ke versi exact; lockfile tetap sama.

## API dan engine

`POST /api/v1/students/{id}/face-preview` menerima `{image: raw_base64_JPEG}`. Go memeriksa live `biometrics.enroll`, scope sekolah/siswa, active status, consent, Origin dan CSRF; kedaluwarsa consent diperiksa lagi setelah inference. JSON maksimal 512 KiB; JPEG maksimal 384 KiB dan dimensi 160–1280. Preview AI internal memakai service token, inference tunggal, dan validasi count/geometry/quality. Respons publik hanya boxes, face_count, quality dan guidance tetap; metadata identitas upstream dibuang. Lihat `docs/openapi/biometrics.json`.

`services/ai/app/face_engine.py` memuat detector/recognizer/matcher/session reusable. Exact cosine vectorized mendukung 1–5 template per identity, margin terhadap identity berbeda, top-K internal, rejection unknown, konfirmasi default tiga dalam lima frame, reset dan cooldown tanpa mengembalikan identity lama. Error PAD, hasil PAD invalid/false dan frame invalid/stale membersihkan konfirmasi. Threshold/margin harus disediakan eksplisit. Galeri dibatasi 3.000 identity/10.000 template dan sesi dibatasi; OpenCV thread default dua, dapat dikonfigurasi 1–16. Model SHA dan encrypted-template AAD memakai pipeline existing. Caller wajib menyediakan galeri satu sekolah dengan consent live.

Enrollment database existing tetap satu encrypted pooled vector per siswa; dukungan multi-template tersedia pada interface engine tanpa migration baru. Engine tidak digunakan untuk memberikan success absensi pada runtime yang belum dievaluasi.

`app.engine_tools` menyediakan benchmark CPU sintetis, calibration sweep dan evaluasi manifest authorized. Manifest memakai anonymous IDs, split calibration/holdout, model SHA, gallery JPEG serta probe tracks 3–32 frame. Gallery/probe overlap dan duplikasi ditolak. Keputusan dan latency berasal dari engine; laporan tidak mengesahkan attendance. Attack/PAD memerlukan evaluasi terpisah. Detail CLI dan format manifest ada di `services/ai/README.md`.

## Verifikasi

- `make check`: ESLint tanpa warning, Prettier, Nuxt typecheck, Go vet/gofmt/race, Ruff; **64 pytest, 30 Node tests dan satu Python script test lulus**.
- `make db-test`: assertion SQL rollback dan race suites auth/masterdata/attendance pada PostgreSQL lokal lulus; schema test terisolasi. Preview menutup akses saat revoke/expiry, Origin/CSRF invalid, role salah, student inactive dan AI response invalid.
- Nuxt production build dan binary Go berhasil; image AI dibangun dari Dockerfile/lockfile. Ketiga runtime lokal diperbarui pada web3001/API8080/AI8001.
- Smoke actual YuNet/SFace dengan gambar kosong sintetis: readiness healthy, preview `no_face`, tidak ada identity/embedding, recognition false. Benchmark tersimpan `.tools/login-camera/engine-benchmark.json`; stage blank/noise/random vectors bersifat sintetis, bukan akurasi maupun FPS kamera nyata.
- Browser: login expired notice, validasi form kosong, login akun pengembangan existing, theme, desktop1280×900 dan mobile390×844; modal enrollment dengan consent unavailable mematikan kamera/capture/save; footer batal dapat dijangkau pada mobile, tanpa overflow horizontal. Screenshot `.tools/login-camera/login-desktop.png`, `enroll-desktop.png`, `enroll-mobile.png`. Tidak mengambil foto atau membuat consent manusia.
- Review memperbaiki exception PAD, reset preview setelah lima frame, pergantian siswa, serta reset konfirmasi saat frame invalid/stale. Regression tests menggunakan doubles/synthetic vectors, bukan wajah nyata.

## Batas dan operasi

Kamera fisik, foto enrollment nyata, dataset hold-out independen, operating thresholds, tracking continuity dan PAD belum diuji. Pengenalan otomatis dan akurasi produksi belum dinyatakan lulus. UI enrollment tersedia setelah persetujuan wali berlaku; screenshot demo sengaja menunjukkan gate tersebut.

Model, virtualenv dan secret ignored. Preview AI memakai image `localhost/attendance-ai:login-camera` pada additional image store `/var/tmp/attendance-ai-build/storage`; pertahankan storage itu selama container memakai image. Untuk restart gunakan command tambahan pada `docs/LOCAL-RUNBOOK.md`. Home hampir penuh; pengaturan ini khusus preview lokal. Coolify/HTTPS/backup produksi tidak termasuk pekerjaan ini.
