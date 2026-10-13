# Panel — shadcn-vue dashboard-01

Arahan pengguna: panel memakai block `dashboard-01`, seluruh kontrol memakai komponen shadcn, dan halaman pencatatan absensi nantinya terpisah dari panel.

## Implementasi

- `/admin/**` memakai layout `admin`: `SidebarProvider`, `AppSidebar` variant inset, `SidebarInset`, `SiteHeader` dan breadcrumb. Sidebar responsif memakai Sheet di mobile, menutup setelah navigasi, dan dapat dibuka/tutup melalui trigger atau shortcut bawaan.
- Komponen block resmi `AppSidebar`, `NavMain`, `NavUser`, `SiteHeader`, `SectionCards` diadaptasi untuk identitas sekolah, sesi aktual dan menu yang tersedia. Users/audit tetap hanya untuk super admin. Logout memakai API dan cookie/session yang sama.
- Forms memakai Field, Input, Select, Checkbox, Switch; feedback memakai Alert, Skeleton, Spinner, Empty; tabel memakai Table, Badge dan Pagination. UI menggunakan token reka-nova neutral yang sudah terpasang, font lokal dan ikon Lucide yang sudah menjadi dependency project.
- Data contoh block, menu billing/notification, grafik revenue dan tabel dokumen demo tidak diterbitkan sebagai data sekolah. Kartu memakai identitas akun/sekolah aktual; grafik kehadiran menunggu data WP-04/05. Dependencies chart/TanStack demo dibuang karena tidak dipakai pada tahap ini.
- SidebarInset sudah menyediakan satu landmark main; konten di dalamnya menggunakan div. Form/tabel mempunyai label, caption, loading/error/empty/disabled states. Header, menu dan pagination berbahasa Indonesia. Sidebar offcanvas yang ditutup diberi inert agar tautan tersembunyi tidak menerima fokus keyboard.

## CLI dan dokumentasi

Context7: resolve `shadcn-vue`, lalu query `/websites/shadcn-vue` dan `/unovue/shadcn-vue` untuk CLI, sidebar dan controlled form components. Registry artifact dan CLI terbaru diperiksa langsung; versi CLI saat pemasangan adalah `2.8.2`.

Command dokumentasi `add dashboard-01` mengembalikan 404 pada registry style `reka-nova`. Source resmi menyediakan block di `new-york-v4`, sehingga dipasang dengan URL registry resmi melalui CLI (didukung dokumentasi CLI), dengan primitives tetap mengikuti konfigurasi proyek:

```bash
cd apps/web
npx shadcn-vue@2.8.2 add https://shadcn-vue.com/r/styles/new-york-v4/dashboard-01.json --yes --overwrite
npx shadcn-vue@2.8.2 add field pagination switch empty breadcrumb spinner --yes --overwrite
```

CLI pertama memasang dependencies primitives seperti sidebar, sheet, avatar, dropdown-menu, separator, tooltip, skeleton, select dan checkbox. Komponen block dipakai di layout/page yang sudah ada; halaman demo yang dihasilkan CLI pada `apps/web/pages/dashboard` dikeluarkan dari aplikasi Nuxt 4 yang menggunakan `app/pages`. Lockfile dan dependency versions tetap exact; tidak ada dependency runtime baru yang dibutuhkan setelah adaptasi.

Referensi primer: [blocks resmi](https://shadcn-vue.com/blocks), [CLI](https://shadcn-vue.com/docs/cli), [sidebar](https://shadcn-vue.com/docs/components/sidebar), [source dashboard-01](https://github.com/unovue/shadcn-vue/tree/dev/apps/v4/registry/new-york-v4/blocks/dashboard-01).

## Halaman absensi berikutnya

Pencatatan absensi/kiosk direncanakan pada `/absensi`, dengan layout mandiri, tanpa sidebar atau header admin, pada WP-08/UI-06. Device session, pairing, kamera dan fallback mengikuti rencana induk; perubahan ini belum membuat endpoint kamera atau menampilkan siswa pada kiosk. Daftar, koreksi dan laporan absensi untuk petugas tetap berada di panel dan memakai izin user. Tidak ada link menu menuju fitur yang belum tersedia.

## Verifikasi

Lint, typecheck, build Nuxt, checks lokal dan build image dijalankan. Browser memeriksa desktop 1440×900 dan mobile 390×844: sidebar, menu akun/logout, breadcrumb, Select peran, Checkbox grants, pencarian/empty state, audit dan overflow tabel. Bukti screenshot lokal di `.tools/dashboard-desktop.png` dan `.tools/dashboard-mobile.png`. Verifikasi terakhir 2026-10-13: logout lewat menu akun berhasil kembali ke login; sidebar mobile menutup setelah navigasi; satu landmark main; lebar scroll halaman sama dengan lebar viewport. Lint/typecheck exit 0, image dashboard berhasil dibangun, dan audit npm tetap 19 temuan upstream (14 high, 5 moderate, 0 critical). Workflow CI remote belum dijalankan.
