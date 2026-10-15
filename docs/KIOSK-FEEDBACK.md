# Absensi kamera dan konfirmasi siswa

Halaman `/absensi` memakai kamera Fantech yang dipilih pada komponen kamera bersama. Siswa bergantian menghadap kamera, menunggu nama dan konfirmasi berhasil, kemudian memberi tempat untuk siswa berikutnya. Kamera tetap berjalan tanpa modal yang harus ditutup guru.

Notifikasi Sonner dan panel hasil menampilkan nama, status Hadir/Terlambat, serta waktu tersimpan. Pemindaian berulang tidak menumpuk notifikasi atau mengubah waktu pertama. Siswa yang kembali setelah meninggalkan area kamera tetap mendapat konfirmasi. Riwayat perangkat dibatasi lima hasil terakhir; riwayat lengkap tetap di laporan. Status yang diatur petugas dan koreksi yang belum selesai tidak ditimpa kamera.

Pada **Jadwal & kalender**, isi **Mulai absen pulang** setelah batas tepat waktu beserta toleransi, dan sebelum tutup absensi. Sesi baru menyimpan salinan jadwalnya. Sebelum jam itu kamera mencatat masuk; sesudahnya kamera mencatat pulang untuk siswa yang sudah masuk. Tanpa jam masuk, sistem meminta bantuan petugas. Jadwal lama tanpa jam pulang tetap hanya mencatat masuk sampai diatur; perubahan jadwal tidak mengubah sesi dan riwayat yang sudah dibuat. Status kehadiran Hadir/Terlambat tetap dipertahankan saat penanda Sudah pulang ditampilkan bersama jam keluar.

Nama pada kotak wajah memakai kecocokan baru pada setiap frame yang diproses. Jeda dua detik setelah konfirmasi dihapus. Saat identitas yang sudah terverifikasi masih cocok tetapi pemeriksaan wajah langsung perlu diulang, nama tetap terlihat dengan penanda kuning; respons ini tidak boleh menulis absensi. Wajah berbeda, tidak ada wajah, kualitas buruk, galeri berubah atau sesi berakhir membersihkan identitas. Skor yang ditampilkan adalah cosine similarity, bukan persentase akurasi.

Waktu penulisan berasal dari database; jam halaman mengikuti waktu server dan zona waktu sesi. Batas sesi perangkat sepuluh menit tetap berlaku, dan waktu berakhirnya terlihat pada halaman.

## Referensi

Komponen dan notifikasi diperiksa melalui Context7 dan MCP shadcn-vue: [Sonner](https://shadcn-vue.com/docs/components/sonner), Card, Badge, Progress dan Select yang sudah terpasang. Tidak ada framework pengenalan atau dependensi baru. Konsep skor dan kesinambungan tracking merujuk [OpenCV SFace](https://docs.opencv.org/4.5.5/d0/dd4/tutorial_dnn_face.html) dan [NVIDIA tracking](https://docs.nvidia.com/metropolis/deepstream/dev-guide/text/DS_plugin_gst-nvtracker.html); implementasi tetap memakai engine CPU yang sudah ada.

## Bukti uji lokal

Sesi uji Fantech `f76dd8c8-963b-413c-83b5-84f4e56abd9b` mencatat siswa terdaftar masuk **16:21:33** dan pulang otomatis **16:23:00**, Asia/Jakarta, 15 Oktober 2026. Waktu masuk tidak berubah pada pemindaian berikutnya. Sesi ditutup dan jadwal asli dipulihkan. Sesi uji sebelumnya juga membuktikan pemindaian saat jam pulang tanpa jam masuk tidak membuat kehadiran palsu.

Test unit antrean memakai 100 pergantian siswa sintetis untuk memeriksa notifikasi, pencegahan duplikasi dan batas riwayat. Test browser notifikasi memakai kamera dan respons pengenalan sintetis khusus presentasi; itu bukan bukti akurasi wajah. Foto/video holdout terpisah, serangan presentasi fisik dan uji beban antrean nyata masih belum dievaluasi.

Sesi notifikasi Fantech `e9884663-62e0-4a7d-91d1-c78adbabd425` memperlihatkan Sonner **Absen masuk berhasil** untuk siswa terdaftar pada **20:22:10**, status **Hadir**, 15 Oktober 2026 Asia/Jakarta. Konfirmasi panel, nama, skor dan jam terlihat pada browser nyata; kamera kemudian dihentikan, sesi ditutup dan jadwal asli dipulihkan.

Verifikasi akhir: migration 00012 diterapkan tanpa reset data; Go race integration dengan database terisolasi, 83 pytest, 64 test Node, 5 test skrip Python, Nuxt lint/typecheck/build dan 6 test browser lulus. Snapshot notifikasi desktop/ponsel memakai fixture presentasi, sedangkan catatan waktu di atas berasal dari kamera nyata.
