# Panel — shadcn-vue dashboard-01

Arahan pengguna: panel memakai block `dashboard-01`, seluruh kontrol memakai komponen shadcn, dan halaman pencatatan absensi nantinya terpisah dari panel.

## Implementasi

- `/admin/**` memakai layout `admin`: `SidebarProvider`, `AppSidebar` variant inset, `SidebarInset`, `SiteHeader` dan breadcrumb. Sidebar responsif memakai Sheet di mobile, menutup setelah navigasi, dan dapat dibuka/tutup melalui trigger atau shortcut bawaan.
- Komponen block resmi `AppSidebar`, `NavMain`, `NavUser`, `SiteHeader`, `SectionCards` diadaptasi untuk identitas sekolah, sesi aktual dan menu yang tersedia. Users/audit tetap hanya untuk super admin. Logout memakai API dan cookie/session yang sama.
- Forms memakai Field, Input, Select, Checkbox, Switch; feedback memakai Alert, Skeleton, Spinner, Empty; tabel memakai Table, Badge dan Pagination. UI menggunakan token reka-vega neutral yang dipasang ulang lewat CLI resmi, font Inter lokal dan ikon Lucide yang sudah menjadi dependency project.
- Data contoh block, menu billing/notification, grafik revenue dan tabel dokumen demo tidak diterbitkan sebagai data sekolah. WP-05 kartu memakai roster sesi, hadir termasuk late, late subset, dan not_recorded aktual; grafik kehadiran menunggu data WP-04/05. Dependencies chart/TanStack demo dibuang karena tidak dipakai pada tahap ini.
- SidebarInset sudah menyediakan satu landmark main; konten di dalamnya menggunakan div. Form/tabel mempunyai label, caption, loading/error/empty/disabled states. Header, menu dan pagination berbahasa Indonesia. Sidebar offcanvas yang ditutup diberi inert agar tautan tersembunyi tidak menerima fokus keyboard.

## CLI dan dokumentasi

Context7: resolve `shadcn-vue`, lalu query `/websites/shadcn-vue` dan `/unovue/shadcn-vue` untuk CLI, sidebar dan controlled form components. Registry artifact dan CLI terbaru diperiksa langsung; versi CLI saat pemasangan adalah `2.8.2`.

Command dokumentasi `add dashboard-01` mengembalikan 404 pada registry style `reka-nova`. Source resmi menyediakan block di `new-york-v4`, sehingga dipasang dengan URL registry resmi melalui CLI (didukung dokumentasi CLI), dengan primitives mengikuti konfigurasi proyek saat pemasangan awal:

```bash
cd apps/web
npx shadcn-vue@2.8.2 add https://shadcn-vue.com/r/styles/new-york-v4/dashboard-01.json --yes --overwrite
npx shadcn-vue@2.8.2 add field pagination switch empty breadcrumb spinner --yes --overwrite
```

CLI pertama memasang dependencies primitives seperti sidebar, sheet, avatar, dropdown-menu, separator, tooltip, skeleton, select dan checkbox. Komponen block dipakai di layout/page yang sudah ada; halaman demo yang dihasilkan CLI pada `apps/web/pages/dashboard` dikeluarkan dari aplikasi Nuxt 4 yang menggunakan `app/pages`. Lockfile dan dependency versions tetap exact; WP-03 menambah paket font Inter untuk penyajian lokal.

Referensi primer: [blocks resmi](https://shadcn-vue.com/blocks), [CLI](https://shadcn-vue.com/docs/cli), [sidebar](https://shadcn-vue.com/docs/components/sidebar), [source dashboard-01](https://github.com/unovue/shadcn-vue/tree/dev/apps/v4/registry/new-york-v4/blocks/dashboard-01).

## Penyamaan preset WP-03

Block lama memakai new-york-v4, sementara primitives awal Nova. CLI terbaru memigrasikan nama style lama ke preset baru; untuk menyamakan seluruh ukuran, spacing dan footer kartu, preset resmi Vega diterapkan menyeluruh:

```bash
npx shadcn-vue@2.8.2 add tabs dialog textarea --yes --overwrite
npx shadcn-vue@2.8.2 apply --preset vega --yes
npm install --save-exact @fontsource-variable/inter@5.3.0
```

`components.json` sekarang `reka-vega`. Semua primitives berasal dari CLI; tidak ada style Nova yang disengaja. Font CSS diimpor dari paket lokal, tanpa Google Fonts. Font dan neutral tokens konsisten dengan preset. Header mengikuti tinggi 48px; tabel/form memakai spacing komponen yang sama. Akses keyboard sidebar offcanvas dan teks Indonesia dipulihkan setelah overwrite CLI. Kontrol tambah/edit data akademik dan siswa memakai Sheet; konfirmasi pencabutan penugasan memakai Dialog.

## Halaman absensi berikutnya

Pencatatan absensi/kiosk direncanakan pada `/absensi`, dengan layout mandiri, tanpa sidebar atau header admin, pada WP-08/UI-06. Device session, pairing, kamera dan fallback mengikuti rencana induk; perubahan ini belum membuat endpoint kamera atau menampilkan siswa pada kiosk. Daftar, koreksi dan laporan absensi untuk petugas tetap berada di panel dan memakai izin user. Tidak ada link menu menuju fitur yang belum tersedia.

## Verifikasi

Lint, typecheck, build Nuxt, checks lokal dan build image dijalankan. Browser memeriksa desktop 1440×900 dan mobile 390×844: sidebar, menu akun/logout, breadcrumb, Select peran, Checkbox grants, pencarian/empty state, audit dan overflow tabel. Bukti screenshot lokal di `.tools/dashboard-desktop.png` dan `.tools/dashboard-mobile.png`. Verifikasi terakhir 2026-10-13: logout lewat menu akun berhasil kembali ke login; sidebar mobile menutup setelah navigasi; satu landmark main; lebar scroll halaman sama dengan lebar viewport. Lint/typecheck exit 0, image dashboard berhasil dibangun, dan audit npm tetap 19 temuan upstream (14 high, 5 moderate, 0 critical). Workflow CI remote belum dijalankan.


## WP-04

Menu Jadwal & kalender, Sesi absensi dan Koreksi absensi menggunakan layout/preset Vega yang sama. Tahun/kelas/siswa tetap melalui master data. Form jadwal dan pencatatan memakai Sheet/Field/Input/Select/Checkbox/Switch/Textarea; tindakan sesi dan keputusan koreksi memakai Dialog dengan title/description. API menghitung waktu hadir/grace; panel menampilkan not_recorded sebagai Belum tercatat sampai rekonsiliasi. Pencatatan manual panel ini terpisah dari route kiosk /absensi yang baru masuk WP-08. Audit aktivitas menerjemahkan tindakan sesi, kalender, roster, manual dan koreksi ke Bahasa Indonesia.


## WP-05

Dashboard memiliki filter tanggal/tahun/kelas, kartu KPI, trend tujuh hari yang dapat dibaca sebagai tabel, dan catatan terbaru dengan NIS masked. `/admin/reports` memakai Field/Input/Select untuk tahun/kelas/periode/status/search, server pagination, totals shared dengan dashboard dan CSV, serta link riwayat dari profil siswa. CSV hanya enabled sesudah filter canonical diterapkan; semua hasil filter diekspor dan diaudit. Audit super admin menambah filter date/actor/action/target dan nama pelaku current. Semua primitives tetap Vega; tidak menambah dependency chart pada P0 sesuai plan. Snapshot roster menentukan siswa historis, bukan kelas siswa saat ini.

## Kelanjutan lokal13Oktober2026

Menu perangkat memakai Card/Table/Sheet/Dialog/Field yang sama. Consent ada pada detail siswa untuk officer, dengan actions di bawah description pada mobile. Import CSV ada di daftar siswa, preview dan errors per baris sebelum commit. Kiosk `/absensi` tidak memakai sidebar/admin layout, dengan Field/Select/Button/Card/Alert dan preview video native; tidak menampilkan identitas siswa. Live status dan checkout ada di detail sesi; report/CSV menambahkan waktu pulang tanpa mengubah denominator statistik. Label/filter audit memasukkan guardian/consent/device/import/checkout.
