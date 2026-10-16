# Koneksi perangkat absensi

Perangkat dipasangkan sekali pada browser yang digunakan untuk absensi. Kode pemasangan tetap sekali pakai dan berlaku lima menit. Pemasangan mengakhiri login operator di browser kiosk.

Koneksi disimpan dalam cookie HttpOnly, Secure pada HTTPS, dan SameSite=Lax. ID perangkat bukan kredensial. Server hanya menyimpan hash token acak. Koneksi berlaku 30 hari sejak penggunaan terakhir dan diperpanjang melalui heartbeat; batas akhir penugasan kelas saat pemasangan tetap berlaku dan tidak dapat diperpanjang oleh penugasan pengganti.

Perangkat mengikuti **satu kelas**, berdasarkan sesi yang dipilih ketika dipasangkan. Server mencari satu sesi terbuka dalam rentang waktunya untuk kelas tersebut. Tanpa sesi, koneksi tetap tersimpan dan layar menunggu. Jika beberapa sesi aktif bertumpang tindih, pencatatan berhenti sampai petugas menyisakan satu sesi. Perangkat tidak berpindah kelas secara otomatis. Untuk mengganti kelas, operator memasangkan ulang dengan sesi kelas yang dituju.

Halaman memeriksa koneksi setiap 15 detik. Pergantian sesi menghentikan kamera dan membersihkan hasil siswa sebelumnya. Petugas dapat mengaktifkan kamera setelah sesi siap; izin kamera tetap mengikuti kebijakan browser. Gangguan jaringan menghentikan kamera, mempertahankan koneksi, dan dicoba kembali otomatis. Tidak ada pencatatan offline.

Pencabutan perangkat, pencabutan izin/penugasan operator, penonaktifan akun/sekolah, atau kedaluwarsa koneksi menghentikan akses. Pemeriksaan izin, kelas dan sesi tetap berlangsung setiap permintaan dan setelah inferensi. Cookie yang dihapus, browser berbeda, mode privat, atau perangkat tidak digunakan selama 30 hari memerlukan pemasangan baru.

Migrasi `00016_persistent_devices.sql` wajib dijalankan sebelum API baru. Kredensial lama mempertahankan batas waktu sebelumnya; pasangkan ulang satu kali untuk memakai koneksi persisten. Tidak ada data siswa atau kehadiran yang dihapus oleh migrasi.

## Verifikasi staging

- Pasangkan, muat ulang `/absensi`, lalu tutup dan buka kembali browser yang sama: koneksi tetap dikenali tanpa login operator.
- Tutup sesi: perangkat menunggu. Buka sesi baru kelas yang sama: perangkat mengikutinya tanpa kode baru.
- Dua sesi aktif untuk kelas yang sama: perangkat menunggu dan endpoint capture menolak pencatatan.
- Buka sesi kelas lain: perangkat tidak mengikuti kelas tersebut.
- Putuskan jaringan dan sambungkan kembali: kamera berhenti, koneksi dipulihkan tanpa kode baru.
- Cabut perangkat/izin operator atau biarkan penugasan berakhir: heartbeat dan pengenalan ditolak. Pemberian izin baru tidak menghidupkan kredensial yang telah dicabut.
