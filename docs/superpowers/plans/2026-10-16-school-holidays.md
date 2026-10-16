# Kalender sekolah dan libur Indonesia

User meminta integrasi sumber Indonesian Holidays dan libur sekolah kustom; cuti bersama telah disetujui. Eksekusi langsung pada branch/check-out saat ini, tanpa reset data.

## Alur

Libur nasional dan cuti bersama disinkronkan per tahun ke database melalui backend; tidak mengirim data sekolah/siswa ke penyedia. Override sekolah diprioritaskan, kemudian libur nasional/cuti bersama, lalu weekday jadwal kelas. Override mendukung rentang maksimal 366 hari dan pengembalian satu hari ke kalender otomatis. Data kosong/tidak valid tidak menghapus cache yang sudah ada. Jika tahun belum punya data, UI menjelaskan belum tersedia; sesi otomatis pada tahun itu menunggu data, kecuali tanggal punya pengaturan sekolah eksplisit. Sesi terbuka/kehadiran lama tidak ditulis ulang.

1. Client HTTP standar + tes kontrak response, batas payload/timeout/tahun/empty data. Cache per sekolah/tahun, migrasi14, sinkronisasi harian dan retry gagal berkala, endpoint sync; network di luar transaksi sekolah.
2. Satu aturan kalender untuk create/open/worker. GET tanggal/bulan, PUT rentang atomik dan DELETE override, validasi dan audit; mutasi kalender seluruh sekolah hanya scope sekolah. Regresi DB untuk precedence, worker, unavailable/cache dan rentang.
3. Install Calendar lewat CLI resmi shadcn-vue; SchoolCalendar reusable dengan month view, status sumber, daftar tanggal bulan terkait, dialog rentang, reset/sync. Form Zod, Sonner, aksi kanan atas, race-safe load. Jadwal kelas tetap pada halaman yang ada.
4. Build/check/race dan browser viewport/flow dengan fixture sintetis. Migrasi dan refresh API/web lokal, sinkronisasi data libur publik, dokumentasi OpenAPI/runbook. Tidak menulis data siswa/kehadiran untuk verifikasi.

Review fokus: pengaturan sekolah menang terhadap sync, tidak ada false absence saat data belum tersedia, kegagalan/empty API menjaga cache, tidak ada lock sekolah selama HTTP, rentang lintas tahun/leap day, pergantian bulan/request lama, dan cakupan akses.

## Hasil

Seluruh langkah selesai. make check/build, integrasi DB -race, dan 15 tes browser lulus. Migrasi 14 serta refresh API/web lokal selesai; 25 tanggal 2026 tersimpan, 2027 belum tersedia. Data siswa/wajah/kehadiran/override sekolah tetap. Temuan review sinkronisasi sesudah pencabutan izin diperbaiki dengan tes RED/GREEN dan diperiksa ulang. Panduan: docs/SCHOOL-CALENDAR.md. Log: .tools/holiday-calendar/.
