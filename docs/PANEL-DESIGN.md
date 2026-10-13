# Panel — shadcn-vue dashboard-01

Arahan pengguna: panel menggunakan dashboard-01 dan komponen shadcn-vue sesuai fungsinya, seluruh form Sheet menjadi modal, badge sesuai status, tabel dan tindakan konsisten. Halaman kiosk `/absensi` tetap di luar panel.

## Konfigurasi saat ini

`apps/web/components.json` menggunakan `style: new-york`, Tailwind CSS 4, CSS variables neutral, alias Nuxt `@/components/ui`, font Inter lokal, dan ikon Lucide. Registry Tailwind 4 untuk block dashboard-01 adalah `new-york-v4`. Context7 dan MCP shadcn-vue mengonfirmasi source ini; CLI `view @shadcn/dashboard-01` berhasil setelah preset disamakan. Preset sebelumnya `reka-vega` mengembalikan 404 untuk block tersebut dan menghasilkan ukuran primitive yang berbeda dari contoh block.

Seluruh 26 kelompok primitive yang dipakai dipasang ulang dengan CLI resmi 2.8.2 dan `--overwrite`. Ukuran tombol, input, kartu, tabs, sidebar dan dialog mengikuti source new-york. Font diambil dari paket lokal `@fontsource-variable/inter`; import Google Fonts tambahan dari CLI dihapus. CLI menjadi devDependency dengan pin exact; dependencies dan lockfile tetap terkunci. Enam file sisa preset lama yang tidak diekspor/dipanggil dihapus.

## Shell dan halaman

- `/admin/**` memakai SidebarProvider, AppSidebar inset, SidebarInset, SiteHeader dan Breadcrumb. Lebar sidebar dan tinggi header mengikuti block resmi; content menggunakan spacing responsif 16/24px. Menu disaring oleh izin dan role yang sudah ada.
- AppSidebar, NavMain, NavUser, SiteHeader dan SectionCards mengadaptasi block untuk data sekolah aktual. Dashboard/reports memakai roster, hadir, terlambat, belum tercatat, trend tujuh hari dan catatan terbaru. Tabel trend tetap dipakai; tidak menambahkan grafik/data contoh block.
- Form memakai Field/FieldGroup/Input/Select/Checkbox/Switch/Textarea; feedback memakai Alert/Skeleton/Spinner/Empty. Seluruh tabel memakai Table, header muted, scroll internal, Badge dan TablePagination.
- TableActions memakai DropdownMenu nonmodal, Button ikon dengan nama aksesibel per baris, DropdownMenuItem dan Separator. Link tetap NuxtLink; tindakan edit/pencabutan/koreksi memakai handler dan batas izin yang sama. Pengguna tidak dapat mengedit akun sendiri dari tabel; koreksi pending tetap memblokir pencatatan.
- Pagination berbagi komponen yang sama termasuk perangkat, dengan jumlah/halaman di kiri dan kontrol di kanan saat ruang cukup. Tabel panjang bergulir dalam container dan tidak melebar ke halaman.
- Sidebar mobile menggunakan Sheet internal komponen Sidebar resmi. Navigasi menutup setelah perpindahan route. Offcanvas desktop yang tertutup tetap `inert`; label pembaca layar dan tooltip berbahasa Indonesia dipulihkan sesudah overwrite CLI.
- `/absensi` memakai layout mandiri dengan komponen yang sama. Kontrak pairing, sesi perangkat, kamera dan gate biometrik tidak diubah oleh pembaruan tampilan ini.

## Modal dan fokus

Sembilan form Sheet di siswa, akademik, jadwal, sesi/roster/manual/checkout/koreksi, penempatan, wali, consent dan perangkat sekarang memakai Dialog. Editor tambah/edit pengguna yang sebelumnya inline juga menjadi Dialog. Setiap modal memiliki Title, Description, form, Footer Batal/Simpan, label isian dan batas tinggi viewport dengan scroll untuk form panjang. Penutupan dan input diblokir selama penyimpanan; error server tetap ditampilkan di modal.

Lima konfirmasi (cabut penugasan, consent, perangkat, tindakan sesi dan keputusan koreksi) memakai AlertDialog dengan Cancel. Submit async memakai Button di form yang dikontrol, sehingga tidak ditutup otomatis sebelum validasi/request berhasil. Alasan wajib, CSRF, audit dan aturan pemohon berbeda dari pemberi keputusan tetap mengikuti implementasi sebelumnya.

DialogContent dan AlertDialogContent mengingat elemen pemicu saat dibuka. Jika pemicunya menu item yang akan hilang, fokus kembali ke tombol menu baris yang stabil. Target yang sudah dilepas dan event yang dicegah caller dihormati. Ini memperbaiki fokus yang sebelumnya jatuh ke body setelah pembatalan modal dari dropdown.

## Warna status

Badge bawaan diperluas melalui variant semantic dan CSS variables dengan pasangan warna light/dark; label status tetap terlihat selain warna.

| Variant | Status |
|---|---|
| success / hijau | aktif, hadir, sesi terbuka, disetujui, perangkat/consent berlaku |
| warning / amber | terlambat, menunggu, perlu review, terjadwal, kedaluwarsa |
| info / biru | izin, sakit, lulus |
| destructive / merah | alfa, ditolak, dicabut, persetujuan dicabut, sesi dibatalkan |
| secondary / netral | nonaktif, sesi ditutup, belum tercatat, pindah, di luar periode, nilai tidak dikenal |

Role dan badge konteks laporan tetap memakai variant netral karena tidak mewakili status operasional. `statusVariant` berbagi mapping di seluruh halaman; kiosk menggunakan variant yang sama untuk status perangkat.

## Dokumentasi dan CLI

Context7 `/unovue/shadcn-vue`: konfigurasi Nuxt/Tailwind 4, CLI, theming, Dialog/AlertDialog, Table/DropdownMenu dan kompatibilitas preset block. MCP shadcn-vue: registries, search/view dashboard-01 dan primitives, contoh source block/Dialog, add command dan audit checklist. CLI dijalankan dari `apps/web` agar tidak membuat konfigurasi ganda di root monorepo.

```bash
cd apps/web
npx shadcn-vue@2.8.2 info
npx shadcn-vue@2.8.2 view @shadcn/dashboard-01
npx shadcn-vue@2.8.2 add \
  @shadcn/alert @shadcn/alert-dialog @shadcn/avatar @shadcn/badge \
  @shadcn/breadcrumb @shadcn/button @shadcn/card @shadcn/checkbox \
  @shadcn/dialog @shadcn/dropdown-menu @shadcn/empty @shadcn/field \
  @shadcn/input @shadcn/label @shadcn/pagination @shadcn/select \
  @shadcn/separator @shadcn/sheet @shadcn/sidebar @shadcn/skeleton \
  @shadcn/spinner @shadcn/switch @shadcn/table @shadcn/tabs \
  @shadcn/textarea @shadcn/tooltip --yes --overwrite
```

Block sudah dipasang pada fondasi melalui URL registry resmi dan diadaptasi ke route/data sekolah. Pembaruan ini mengambil ulang primitive yang cocok; demo page dan dependency grafik block tidak dipasang ke runtime. Saat overwrite CLI berikutnya, pertahankan semantic badge, pengembalian fokus modal, atribut inert sidebar, label Indonesia dan font lokal yang dijelaskan di atas.

Referensi: [blocks](https://shadcn-vue.com/blocks), [CLI](https://shadcn-vue.com/docs/cli), [theming](https://shadcn-vue.com/docs/theming), [Dialog](https://shadcn-vue.com/docs/components/dialog), [AlertDialog](https://shadcn-vue.com/docs/components/alert-dialog), [source dashboard-01](https://github.com/unovue/shadcn-vue/tree/dev/apps/v4/registry/new-york-v4/blocks/dashboard-01).

## Verifikasi lokal 13 Oktober 2026

`make check` lulus: lint/typecheck, Go vet/gofmt/race, Python ruff + 8 tests, bootstrap unittest dan 7 tes regresi UI. Dua regresi baru mencakup pemetaan status dan fokus modal (menu trigger, event dibatalkan, target dilepas). Lint/typecheck/build Nuxt sesudah perbaikan akhir dijalankan kembali sebelum runtime web diperbarui. Detector Impeccable tidak menemukan masalah deterministik; pemeriksaan browser tetap diperlukan.

Browser memeriksa login, dashboard, akademik, siswa/detail, jadwal, perangkat, pengguna, sesi/detail, koreksi, laporan, audit dan import. Modal diuji buka/batal, fokus/keyboard dan label; AlertDialog tindakan sesi diuji tanpa submit. Pada mobile 390px, form jadwal muat dalam viewport dan form izin pengguna yang panjang bergulir dalam modal; navigasi menutup setelah route dipilih. Penyimpanan/pencabutan/consent/import tidak dijalankan ulang melalui browser untuk pembaruan tampilan ini. CI remote dan UAT sekolah belum dijalankan. Audit npm saat pemasangan masih 19 advisory upstream (14 high, 5 moderate), bukan status dependency bersih.

Log dan screenshot lokal di `.tools/shadcn-*` (ignored), termasuk hasil CLI, check/build, detector dan bukti UI akhir.
