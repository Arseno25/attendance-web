# Bahasa aplikasi, pengaturan sekolah, dan foto profil

> Pelaksana: gunakan executing-plans secara langsung di sesi ini.

Tujuan: petugas memahami halaman tanpa istilah implementasi, pengelola mengubah identitas sekolah, dan pengguna mengunggah foto profilnya sendiri.

Desain: pertahankan komponen shadcn dan perilaku bisnis. Pengaturan baru `/admin/settings` berisi nama sekolah, nama aplikasi, alamat, telepon, email, dan logo. Zona waktu tetap mengikuti konfigurasi sekolah. Hak `school.write` dan pembatasan pengelola penuh yang sudah ada tetap digunakan. Foto profil berada pada Akun saya dan tampil di menu akun. Upload diberi pratinjau serta tombol simpan/hapus yang jelas; perubahan foto tidak memerlukan perubahan password.

Gambar: gunakan JPG/PNG maksimal 2 MB, batasi dimensi sebelum decode, ubah ke gambar kecil tanpa metadata, simpan binary di PostgreSQL. Foto pribadi hanya dapat dibaca pemilik akun; logo dan nama sekolah tersedia di halaman masuk. Tidak menerima SVG, URL gambar dari pengguna, atau nama file untuk lokasi penyimpanan. Tidak menambahkan penyimpanan eksternal/dependensi.

- [x] API dan migrasi 00010: pengaturan sekolah, identitas publik, logo, dan foto profil. Uji izin/CSRF/isolasi akun, batas file, penggantian/penghapusan, data rusak, dan audit tanpa isi gambar. Pertahankan seluruh pekerjaan RBAC dan enrollment yang sudah ada.
- [x] Tampilan: uploader reusable memakai Field/Input/Avatar/Button, pengaturan sekolah dengan tombol utama kanan atas, integrasi identitas pada sidebar/login/menu akun. Validasi Zod dan notifikasi Sonner sesuai pola aplikasi.
- [x] Bahasa: tinjau teks tampilan, validasi, pesan API, daftar izin dan status; ganti jargon dengan tindakan dan hasil yang dapat dipahami. Nama properti/protokol internal tidak diubah. Rapikan kelompok form dan modal panjang yang ditemukan.
- [x] Verifikasi: seluruh suite Go/database/AI/UI/Python, lint/typecheck/build, backup terenkripsi, migrasi lokal tanpa mengubah data pengguna, refresh runtime dan satu putaran browser desktop/mobile. Tidak mengunggah foto pengguna atau mengubah profil/sekolah nyata saat verifikasi browser.

Batas: tanpa editor/crop foto, tema baru, atau perubahan otorisasi/perhitungan absensi. Pendaftaran wajah lokal dan pembatasan absensi otomatis tetap berlaku.

Verifikasi: seluruh Go test -race dengan PostgreSQL nyata, 51 tes Node UI, 65 tes AI, dan satu tes Python lolos. Lint/typecheck/build/Go vet lolos. Review memperbaiki orientasi ponsel, pemulihan setelah simpan logo, dan sisa copy. Backup terenkripsi dan migrasi 00010 diverifikasi; data pengguna dipertahankan. Pilihan gambar disimpan melalui tombol eksplisit dan input tersembunyi tidak meluapkan layout mobile. Tidak memakai foto pengguna selama pengujian.

Ruling: identitas publik juga dipakai di halaman awal agar nama/logo tidak berbeda ketika pengguna membuka tautan beranda. Pesan beranda yang masih menyatakan aplikasi belum dibuat diganti dengan petunjuk masuk dan pencatatan manual yang sesuai fitur saat ini.
Ruling: contoh CSV berisi header saja disediakan pada import siswa agar bantuan tidak meminta pengguna memahami nama kolom internal dari teks halaman; aturan parser/header tetap sama.
Ruling: pemilih gambar memakai Button shadcn berbahasa Indonesia dan Input file tersembunyi; input `sr-only` sempat meluber karena lebar bawaan Input, sehingga diganti `hidden`. Browser akhir mengonfirmasi lebar scroll sama dengan lebar halaman pada 390 piksel dan tombol simpan akun berada di kanan atas desktop.
