# Desain RBAC dinamis untuk panel sekolah

- **Status:** Draft untuk review pengguna; belum diimplementasikan.
- **Tanggal:** 14 Oktober 2026.
- **Cakupan:** RBAC role-permission lintas API Go, PostgreSQL, dan panel Nuxt.
- **Keputusan yang sudah disetujui:** role bawaan dan custom dikelola melalui satu model; superadmin mendapat semua permission secara default dan boleh mengedit permission role Superadmin; izin langsung per pengguna dihapus; cakupan role adalah sekolah atau kelas yang ditugaskan; izin lama dipertahankan lewat role migrasi; hanya superadmin boleh mengelola RBAC.

## 1. Tujuan dan kriteria sukses

Bangun satu sumber otorisasi yang bisa dikelola dari panel dan ditegakkan backend. Superadmin dapat membuat role, memilih permission dari katalog aplikasi, mengubah role yang tersedia termasuk permission role Superadmin, dan menetapkan role tersebut kepada pengguna. Pengubahan tidak boleh menyebabkan privilege escalation, akses kelas/sekolah bocor, atau lockout dari pengelolaan role dan pengguna.

Kriteria sukses:

1. Role dan permission efektif dibaca dari data sekolah di PostgreSQL pada setiap pemeriksaan otorisasi; tidak ada permission role operasional yang hanya bersumber dari map statis Go.
2. Superadmin aktif dapat mengelola pengguna dan role. Semua permission katalog terpasang pada role Superadmin saat inisialisasi; `users.read`, `users.write`, dan `roles.manage` wajib tetap terpasang. Role ini selalu berscope sekolah dan tidak dapat dihapus atau diganti key-nya.
3. Role selain Superadmin hanya mendapat permission eksplisit yang dipilih dari katalog allowlist aplikasi. Role dapat memiliki scope sekolah atau kelas yang ditugaskan.
4. Setiap izin langsung lama dipertahankan melalui role migrasi yang tidak memperluas akses; tidak ada lagi API atau UI untuk menyetel grants langsung pada pengguna.
5. Semua endpoint dan query yang memuat data sekolah/kelas menerapkan permission dan scope dari role dinamis. UI hanya menyembunyikan navigasi; bukan batas keamanan.
6. Perubahan role/permission bertransaksi, masuk audit, dan mencabut sesi pengguna yang terdampak agar sesi/login lama tidak menyimpan akses sebelumnya.

## 2. Model otorisasi

### Role

Role adalah entitas per sekolah dengan UUID internal, `key` stabil, nama tampilan, deskripsi opsional, `scope` (`school` atau `assigned_classes`), penanda role sistem, waktu pembuatan/perubahan, dan daftar permission. `users.role` tetap menyimpan key role untuk mengurangi perubahan kontrak yang sudah dipakai frontend/API, tetapi mendapat foreign key `(school_id, role)` ke role pada sekolah yang sama. Key immutable setelah dibuat; nama tampilan boleh diubah. Role dan user tidak boleh menyeberang tenant sekolah.

Empat role saat ini (`super_admin`, `admin`, `teacher`, `operator`) dipertahankan sebagai role bawaan dengan key stabil. `super_admin` berscope `school`, tidak bisa dihapus, dan tidak dapat kehilangan permission inti. Role bawaan selain itu dapat diedit dan tidak bisa dihapus; role custom dapat dihapus hanya jika tidak dipakai akun mana pun. Penghapusan role yang masih dipakai menghasilkan `409 CONFLICT`, termasuk bila pengguna nonaktif masih mengacu kepadanya.

### Katalog permission

Katalog ditentukan aplikasi sebagai allowlist terpusat dan disajikan melalui API; superadmin tidak membuat identifier permission arbitrer. Implementasi mengonsolidasikan permission yang sudah dikenali kode saat ini dan menambahkan `roles.manage`. Set minimum berdasarkan `rolePermissions` lama:

| Grup | Permission |
|---|---|
| Pengguna & keamanan | `users.read`, `users.write`, `roles.manage`, `audit.read`, `school.write` |
| Akademik & siswa | `students.read`, `students.write`, `academics.read`, `academics.write` |
| Kehadiran | `attendance.read`, `attendance.manual`, `attendance.approve`, `corrections.request` |
| Laporan | `reports.export` |
| Perangkat | `devices.manage`, `devices.health` |
| Biometrik & persetujuan | `biometrics.enroll`, `biometrics.revoke`, `consents.manage` |

Setiap entri katalog mempunyai key, label Indonesia, deskripsi singkat, dan grup. Sumber definisi tunggal Go/API dapat memberi metadata tersebut ke UI. Tes memastikan permission yang dirujuk handler terdaftar dan tiap permission memiliki label/grup. Cakupan aktual tetap berlaku: izin roster/akademik/report yang terkait kelas tidak boleh mengabaikan scope meski permission-nya diberikan.

`roles.manage` dan `users.read`/`users.write` adalah permission inti role `super_admin`. Endpoint pengelolaan RBAC juga memeriksa identitas role sistem `super_admin` secara eksplisit. Maka memberikan `roles.manage` pada role lain tidak memberi hak administrasi role. Superadmin dapat mengubah permission lain pada role Superadmin, tetapi role tersebut tetap mempunyai semua permission awal hanya sampai admin mengeditnya; setelah pengubahan, tiga permission inti tetap wajib. Role sistem tetap berscope sekolah.

### Permission efektif dan scope

Permission efektif pengguna adalah permission pada role yang direferensikan pengguna. Tidak ada union dengan `permission_grants` atau sumber izin per-user lainnya. Superadmin tidak dibypass oleh keputusan individual `rolePermissions` yang tersebar; exception sistem yang tersisa hanya guard identitas untuk operasi khusus superadmin dan pengecekan scope.

Scope `school` melihat data yang tersedia bagi permission-nya di sekolah terkait. Scope `assigned_classes` hanya melihat/mengubah data untuk kelas yang memiliki penugasan aktif dan berada dalam tahun ajaran aktif serta periodenya berlaku. Tanpa penugasan aktif, role kelas tidak memperoleh data kelas. Scope adalah properti role, bukan permission; semua query class-scoped membatasi baris terlepas dari halaman UI.

Perubahan permission berlaku kepada semua pengguna role tersebut. Perubahan `role`, `scope`, atau permissions mencabut sesi seluruh pengguna role terdampak dalam transaksi yang sama. Pengubahan akun pengguna yang mengubah role/status juga mencabut sesi pengguna itu. UI mengarahkan pengguna yang terkena pencabutan untuk login kembali sebagaimana perilaku perubahan akses saat ini.

## 3. Migrasi dan kompatibilitas data

Tambahkan migration Goose baru; jangan mengedit migration yang sudah terpasang. Migration harus bisa dijalankan pada database existing dan menghasilkan role untuk setiap sekolah serta sekolah yang dibuat setelah upgrade. Bootstrap akun pertama menyiapkan role bawaan sebelum/bersamaan dengan membuat pengguna superadmin.

Urutan logis migration:

1. Buat tabel role sekolah, katalog/relasi permission role, constraints uniqueness/FK tenant, dan index yang diperlukan. Seed role bawaan per sekolah existing; bootstrap/initialization menangani sekolah baru.
2. Seed permission role bawaan dari kebijakan versi sekarang yang telah direview. Superadmin baru diberi semua permission katalog; permission inti diwajibkan constraint aplikasi pada setiap update.
3. Untuk tiap akun non-superadmin, hitung permission efektif lama menggunakan `rolePermissions` serta `permission_grants` lama yang valid, dan tentukan scope legacy yang sama. Akun tanpa perubahan akses tetap memakai role bawaan.
4. Kelompokkan akun yang mempunyai tuple sama `(role lama, scope legacy, himpunan permission efektif)` ke satu role migrasi stabil. Jangan menggabungkan kombinasi permission berbeda, jangan melebur izin ke role bawaan bersama bila akan memperluas akses, dan jangan memindahkan pengguna ke role berscope sekolah ketika sebelumnya terbatas kelas. Role migrasi memakai nama yang jelas, misalnya `Migrasi · Guru · <suffix stabil>`, dan dapat ditinjau/diedit superadmin setelah upgrade.
5. Pindahkan assignment lama beserta UUID, rentang waktu, status, dan riwayatnya ke tabel penugasan pengguna/kelas generik. Scope migrasi untuk teacher tetap kelas; role legacy lain mengikuti semantik akses sebelum migrasi. Akun dengan scope kelas tetapi belum memiliki penugasan tetap tidak memperoleh kelas.
6. Setelah verifikasi FK dan jumlah baris, hapus data `permission_grants`, hilangkan batas CHECK role statis, lalu pasang FK role dinamis. Data audit historis tetap tidak diubah; event lama mempertahankan action/target type aslinya.

Migrasi mengelompokkan kombinasi permission identik agar tidak membuat satu role per pengguna. Role migrasi ber-key stabil yang berasal dari source role dan signature permission/scope; collision ditangani secara deterministik. Akun superadmin memakai role sistem Superadmin dan semua permission baru; tidak ada grant lama yang dapat menurunkan hak atau memberi role lain akses administrasi.

Migration diuji dari schema yang memuat empat tabel/kolom auth/assignment saat ini dengan fixture kombinasi grant berbeda, akun aktif/nonaktif, role bawaan, assignment revoked dan assignment valid. Uji membuktikan hasil efektif tiap akun sama atau lebih sempit hanya jika kontrak permission lama tidak valid; tidak boleh ada akses tambahan akibat migrasi. Run migration pada database preview operasional hanya setelah implementasi, uji disposable lulus, dan verifikasi/backup sesuai runbook; jangan menjalankan reset atau downmigration pada data pengguna.

## 4. Penugasan kelas generik

Generalisasi tabel `teacher_assignments` menjadi `user_class_assignments` dengan rename semantik `teacher_id` menjadi `user_id`, mempertahankan UUID, school/class/year, rentang, flag aktif, waktu revoke, constraint interval, dan histori. Perbarui foreign key serta trigger sehingga target harus pengguna aktif dengan role berscope `assigned_classes`, bukan semata role bernama `teacher`. Assignment hanya berlaku pada sekolah yang sama dan kelas/tahun yang valid.

API menggunakan `/api/v1/class-assignments`, `/api/v1/class-assignments/{id}`, dan `/api/v1/class-assignment-users`; payload memakai `user_id`, `class_id`, dan periode yang sudah divalidasi terhadap tahun ajaran. Pilihan target hanya pengguna aktif dengan role berscope kelas. Pencabutan tetap append-preserving: ubah status/periode, jangan hapus row. Endpoint opsi memerlukan permission `academics.write`; melihat/mengelola assignment mengikuti pola `academics.read`/`academics.write` saat ini. Audit baru memakai target `user_class_assignment`; audit lama tidak ditulis ulang.

Halaman akademik memakai ulang tab penugasan saat ini, berganti label menjadi `Penugasan kelas` dan memilih pengguna, bukan hanya guru. Ketika scope role pengguna berubah ke `school`, assignment lama disimpan sebagai histori tetapi tidak dipakai untuk membatasi role tersebut. Bila scope kemudian kembali ke `assigned_classes`, hanya assignment yang masih aktif dan dalam periode berlaku yang memberi akses.

## 5. API Go

Pertahankan envelope JSON, CSRF/Origin untuk mutasi, format error, `request_id`, middleware/session dan pola transaksi proyek.

| Method | Endpoint | Akses | Fungsi |
|---|---|---|---|
| GET | `/api/v1/permissions` | Superadmin | Katalog permission berlabel dan bergrup |
| GET | `/api/v1/roles` | Superadmin | Role sekolah, scope, system flag, permissions, jumlah pengguna |
| POST | `/api/v1/roles` | Superadmin | Buat role dengan nama, deskripsi, scope, permission keys |
| PATCH | `/api/v1/roles/{id}` | Superadmin | Ubah metadata, scope dan permission role |
| DELETE | `/api/v1/roles/{id}` | Superadmin | Hapus role custom yang tidak dipakai |

Response role memuat `id`, `key`, `name`, `description`, `scope`, `is_system`, `permissions` (string[]), dan `user_count`. Permission memiliki `key`, `label`, `description`, dan `group`. API mengurutkan hasil secara stabil; response list non-null array.

Validasi backend: nama setelah trim wajib dan dibatasi 120 karakter; deskripsi opsional dibatasi 500; scope harus salah satu enum; permission harus unik dan seluruhnya ada di katalog; role Superadmin harus menyimpan permission inti dan scope sekolah. Role key dibuat server-side dari nama dengan suffix collision-resilient, immutable, dan tidak dipakai sebagai otoritas selain lookup role. Nama role tidak boleh duplikat secara case-insensitive pada satu sekolah. Request dengan field tidak dikenal mengikuti decoder JSON ketat.

POST sukses `201`, PATCH `200`, DELETE `204` atau envelope konsisten proyek, invalid `400 VALIDATION_ERROR`, akses `403 FORBIDDEN`, referensi/penghapusan terpakai `409 CONFLICT`, tidak ada data sekolah lain. Mutation menulis audit (`roles.created`, `roles.updated`, `roles.deleted`) tanpa merekam body rahasia; metadata mencakup perubahan nama/scope dan permission keys lama/baru agar perubahan keamanan dapat ditinjau. Kegagalan audit membatalkan transaksi.

`POST /users` dan `PATCH /users/{id}` tetap menerima `role` sebagai role key yang valid di sekolah. Hapus `grants` dari request dan response user; `User` tidak mengandung grants. `GET /users` dapat menyertakan label role/scope, atau frontend memetakan melalui koleksi role yang sudah diambil. Session tetap mengembalikan `permissions` efektif untuk shell/navigasi. Role yang tidak ditemukan pada sekolah tersebut ditolak sebagai validasi, bukan menciptakan role implisit.

`BeginAuthorized` dan load session membaca role/scope/permissions efektif dari DB. Pembacaan permission dan scope dikelompokkan/diindeks untuk mencegah query N+1. Pemeriksaan mutasi mengambil snapshot live dalam transaksi setelah lock sekolah/pengguna sebagaimana pola sekarang. RBAC endpoints melakukan pemeriksaan role `super_admin` secara eksplisit di backend. Audit, pengguna, dan permission catalog tidak dapat diakses role custom meskipun role diberi key permission terkait.

## 6. Penegakan seluruh endpoint dan query

Hilangkan ketergantungan akses yang tersebar pada nama `teacher`, `admin`, dan `operator`. Ganti dengan permission eksplisit dan `scope` efektif. Tetapkan permission yang diperlukan tiap handler melalui daftar permission allowlist yang merujuk katalog. Uji/review setiap call-site `BeginAuthorized` serta query scope di:

- auth/session/policy, users, roles, audit, profil;
- data siswa, consent/wali, biometrik, akademik, kelas dan assignment;
- sesi absensi, roster, catatan, checkout/koreksi dan perangkat;
- dashboard/summary, reports, CSV export dan websocket/realtime.

Query dengan data kelas menggunakan `user_class_assignments` aktif untuk daftar class ID aktor dan membatasi row terhadap daftar tersebut. Untuk write, backend memverifikasi target class/resource berada dalam scope yang sama di transaksi, bukan hanya menyembunyikan pilihan di UI. Role berscope sekolah tidak mendapatkan akses di luar permission-nya. Role kelas tidak mendapat akses sekolah hanya karena punya `students.read`, `attendance.read`, atau `reports.export`. Semua query tetap berfilter `school_id`; jangan menerima tenant identity dari request client.

Aturan khusus `attendance.manual` yang saat ini dapat memberi akses roster/correction terbatas harus dipertahankan secara eksplisit melalui permission katalog dan scope kelas; jangan membiarkan pengecualian tersembunyi melewati permission baru. Scope kelas berlaku untuk data terkait kelas, sedangkan endpoint seperti health perangkat tetap dikendalikan permission fitur.

## 7. UI Nuxt/shadcn-vue

Tambahkan item **Role & permission** di grup Administrasi, hanya untuk `session.user.role === 'super_admin'`; routing memakai layout/middleware `auth` + `super-admin` seperti audit/pengguna. Backend tetap sumber enforcement. Jangan menambah dependency atau mengganti primitive desain panel.

`/admin/roles` mengikuti konvensi `PageHeader`, Card, DataTable, Badge, TablePagination jika diperlukan, Dialog untuk tambah/edit, Field/Select/Checkbox, Alert dan Sonner. Tabel menampilkan nama, scope, jumlah pengguna, jumlah permission dan tindakan. Superadmin diberi penanda role sistem. Editor menampilkan nama/deskripsi, scope dan permission terkelompok dengan label/deskripsi katalog. Permission inti Superadmin ditandai wajib/non-editable; permission lain pada role Superadmin tetap bisa diedit. Ringkasan menegaskan perubahan berlaku ke semua pemegang role dan sesi mereka perlu login ulang.

Role edit memakai optimistic state tidak boleh mengubah tampilan seolah tersimpan bila API gagal. Tombol submit terkunci saat request; validasi inline; error API tetap terlihat; simpan sukses toast; modal tetap terbuka saat gagal dan menutup saat sukses; fokus modal kembali sesuai pola aplikasi. Hapus role memakai AlertDialog yang menjelaskan larangan jika role sedang dipakai. Loading/error/empty state, keyboard, label aksesibel, dark mode dan lebar viewport mobile mengikuti primitive/pola proyek.

Tambahkan bagian manajemen role tersendiri pada navigasi/halaman pengguna yang sudah ada tanpa mengubah alur profil pengguna: pilih role dinamis dari API, tampilkan label yang bersumber dari data role, buang checkbox grants serta teks izin tambahan. Penggantian role memicu pesan bahwa semua permission berasal dari role dan sesi akun akan dicabut.

## 8. Rencana pengujian dan kriteria penerimaan

### Go dan database

- Unit policy: permission berasal dari role; role unknown deny-by-default; role Superadmin mendapat semua permission awal; inti wajib dan scope sekolah tak bisa dilepas; scope assigned class menolak kelas tanpa assignment; permission tambahan tidak mengubah scope.
- Integration auth: GET katalog/role serta create/update/delete hanya superadmin; non-superadmin ditolak walau cookie/CSRF valid; invalid/duplikat permission, scope/key/nama ditolak; school isolation; role terpakai tidak bisa dihapus; transaksi audit dan mutation atomik; perubahan role permissions mencabut semua sesi pengguna terdampak.
- Integration users: tidak menerima/mengembalikan grants; hanya key role existing dapat ditetapkan; perubahan user role/status mencabut sesi; bootstrap sekolah baru menyiapkan built-ins.
- Migration: uji upgrade terhadap data lama di PostgreSQL disposable, memastikan built-ins, role migrasi berdasarkan permission+scope, assignment/histori dan referensi user tetap valid, tabel grants dikosongkan/dihilangkan, dan tidak ada permission gabungan lintas pengguna.
- Domain: tes scope pada siswa, akademik, assignment, roster, attendance, corrections, reports/export, biometrik, devices, websocket; role custom yang setara permission/scope berjalan seperti yang dipilih; role tanpa permission atau scope yang tepat ditolak.
- Regression: pertahankan aturan class/year validity, audit append-only, CSRF, revokasi consent/template, session locking, dan perilaku superadmin terakhir.

### Web

- Tes komponen/helper untuk sidebar superadmin vs non-superadmin dan tabel/editor dengan permission katalog dinamis.
- Source test untuk create/edit/cancel/error, validasi selection, mandatory permissions role Superadmin, delete conflict, dan update user tanpa grants.
- `npm run lint`, `npm run format:check`, `npm run typecheck`, serta `npm run build` dari `apps/web`.
- Integrasi browser lokal: role dibuat, diedit dan dimuat ulang; role dipakai untuk pengguna; akses berubah setelah login ulang; akun non-superadmin gagal mengakses route/API; scope kelas diuji lintas pengguna; tidak ada overflow horizontal pada viewport mobile.

### Gate akhir

Jalankan `make check`, `make build`, dan tes migration/integration terhadap database disposable. Jalankan smoke browser terhadap kontrak API serta alur panel. Jangan menyebut UAT, produksi, atau migrasi database aktif berhasil sebelum benar-benar dijalankan dan diperiksa. Catat pemeriksaan yang tak dapat dijalankan sebagai keterbatasan, jangan melemahkan tes/policy untuk membuatnya lulus.

## 9. Batas cakupan

Tidak mencakup permission builder untuk identifier baru, permission override per pengguna, role global lintas sekolah, hirarki/role inheritance, delegasi administrasi role kepada non-superadmin, MFA baru, perubahan provider auth, atau perubahan runtime biometrik. Tidak ada migrasi langsung di environment aktif sebelum backup/verifikasi dan persetujuan terpisah yang diperlukan. Semua halaman tetap berbahasa Indonesia dan memakai stack/pola proyek yang tersedia.
