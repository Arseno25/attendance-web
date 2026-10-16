> Pembaruan implementasi 15 Oktober 2026: alur wali/persetujuan wajah telah dihentikan sesuai keputusan pemilik proyek (migrasi 11). ID persetujuan lama hanya dipertahankan sebagai konteks enkripsi template lama. Bagian blueprint berikut yang masih menyebut alur tersebut adalah rancangan awal; kontrak aktif mengikuti kode/OpenAPI dan [runbook staging](docs/STAGING-RUNBOOK.md).

# 01 — Product Requirements Document (PRD)

## Metadata

- **Produk:** School Face Attendance
- **Versi dokumen:** 1.0 (12 Oktober 2026)
- **Platform:** Responsive Web Admin, kiosk web kamera
- **Target:** Satu sekolah (MVP), arsitektur dapat diperluas
- **Status:** Draft untuk engineering/legal/design review

## 1. Masalah dan nilai

Absensi manual memerlukan waktu, mudah terjadi keterlambatan rekap, dan berisiko input ganda. Produk menawarkan pencatatan real-time dan laporan terpusat. Karena pemrosesan biometrik anak berisiko tinggi, **sistem tidak boleh bergantung secara eksklusif pada AI**: sediakan jalur manual yang setara dan audit koreksi.

## 2. Tujuan bisnis & non-tujuan

**Goals:** (G-1) absensi masuk otomatis tanpa input identitas; (G-2) satu sumber kebenaran data siswa/kelas; (G-3) laporan yang dapat diaudit; (G-4) meminimalkan salah identifikasi, duplikasi, retensi berlebihan; (G-5) operasi kiosk sederhana.

**Non-goals MVP:** monitoring CCTV umum, pelacakan lokasi siswa, analisis emosi/demografi, pengenalan di ruang publik, penilaian disiplin otomatis dari AI, multi-sekolah SaaS, mobile app native, integrasi pembayaran, WhatsApp mass messaging, model AI dilatih dari biometrik sekolah, pencarian visual bebas oleh guru.

## 3. Persona & akses

| Persona | Tugas | Akses |
|---|---|---|
| Super Admin Sekolah | pengguna, konfigurasi, audit, pengelolaan siswa | Scope satu sekolah |
| Admin Tata Usaha | CRUD siswa/kelas, import, absensi, laporan | Sesuai izin RBAC |
| Operator Kiosk | registrasi dengan izin, kesehatan perangkat, review kegagalan | Terbatas, biometrik sensitif dibatasi |
| Guru/Wali Kelas | lihat absensi kelas, ajukan koreksi, ekspor terbatas | Hanya kelas/tahun ajar ditugaskan |
| Siswa | datang di area absensi atau memilih jalur alternatif | Tidak perlu akun kiosk |
| Orang Tua/Wali | informasi, persetujuan/penarikan via proses terverifikasi | Tidak wajib portal MVP |

## 4. User journeys utama

### Journey J-1: Data siswa

Admin login → halaman Siswa → tambah / import → validasi NIS unik dalam sekolah → pilih kelas/tahun ajar → simpan → daftar diperbarui. NISN bersifat opsional sesuai data yang diperlukan; tidak perlu menampilkan data berlebihan.

### Journey J-2: Enrollment biometrik

Admin memilih siswa → cek *legal gate* (dasar pemrosesan, pemberitahuan, status persetujuan wali yang sesuai) → capture beberapa sampel → quality & liveness check → cross-check duplikat → petugas review bila ambigu → enkripsi template → aktifkan di gallery → audit. Tolak proses jika consent ditarik atau tidak valid. Jalur manual tetap tersedia.

### Journey J-3: Absensi otomatis 1:N

Kiosk terdaftar aktif → mendeteksi wajah di zona absensi → memilih frame berkualitas → mengirim gambar sementara ke API → AI melakukan liveness dan exact match gallery → keputusan `accepted`, `unknown`, `inconclusive`, atau `spoof_suspected` → Go validasi status siswa/sesi/device/consent → transaksi idempotent → respons kiosk terbatas + event dashboard.

### Journey J-4: Absensi alternatif / koreksi

Operator memilih siswa setelah verifikasi administratif → alasan dan bukti minimum → rekam event manual + siapa, kapan, alasan → bila kebijakan memerlukan, approver berbeda menyetujui → rekap berubah dengan audit, tidak menghapus histori.

### Journey J-5: Laporan

Guru memilih tahun ajar, kelas, tanggal → melihat hadir/terlambat/izin/sakit/alfa (setelah sesi ditutup) → ekspor terbatas dan terlacak.

## 5. Requirement fungsional

| ID | Requirement | Prioritas | Acceptance criteria |
|---|---|---|---|
| FR-01 | Login admin/operator/guru | P0 | Sesi aman, logout dan timeout; akses dicek server |
| FR-02 | CRUD siswa + status | P0 | NIS unik per sekolah; perubahan terekam audit |
| FR-03 | Tahun ajaran, kelas, penempatan | P0 | Sejarah penempatan tidak hilang saat naik kelas |
| FR-04 | Import CSV/XLSX | P1 | Preview dry-run, validasi baris, rollback bila gagal |
| FR-05 | Device provisioning | P0 | Hanya kiosk terdaftar bisa kirim recognition; bisa revoke |
| FR-06 | Consent dan enrollment biometrik | P0 | Tidak bisa aktif tanpa status legal valid; log pemberian/penarikan |
| FR-07 | Recognition otomatis 1:N | P0 | Tidak butuh NIS/QR; unknown tetap unknown |
| FR-08 | Liveness/quality/multi-frame stabil | P0 | Frame buruk dan inconclusive tidak mencatat kehadiran |
| FR-09 | Attendance masuk/pulang | P0/P1 | Check-in P0; check-out P1; jadwal/hari libur dikonfigurasi |
| FR-10 | Anti-duplicate | P0 | Unik per `(school, session, student)` dalam transaksi concurrent |
| FR-11 | Izin/sakit/absen manual & koreksi | P0 | Membutuhkan alasan, otorisasi, dan jejak audit |
| FR-12 | Dashboard dan laporan | P0 | Filter kelas/tanggal/status, ekspor sesuai role |
| FR-13 | Update event real-time | P1 | Admin melihat attendance baru via WebSocket, fallback refetch |
| FR-14 | Audit log | P0 | Terlacak actor/action/target/time tanpa gambar/raw template |
| FR-15 | Lifecycle biometric | P0 | Revoke & purge dari DB, cache, dan backup berdasarkan kebijakan |
| FR-16 | Akses guru berbasis kelas | P0 | Guru tidak bisa membaca siswa kelas lain lewat direct API |
| FR-17 | Kiosk offline behavior | P1 | Fail-closed untuk biometrik, tidak menyimpan raw image di browser |
| FR-18 | Health, model version & metrik | P0 | Operator tahu kamera/AI/service error, tanpa data PII di log |

## 6. Aturan bisnis

- `school_timezone = Asia/Jakarta` secara default; simpan semua timestamp di PostgreSQL sebagai `timestamptz` UTC dan tampilkan zona lokal.
- Kalender sekolah mengatur hari efektif, libur, waktu check-in/out, batas terlambat dan grace period.
- Status awal `not_recorded`; label `absent` baru disahkan setelah sesi ditutup dan reconciliation selesai. Jangan set `alfa` segera setelah kamera gagal.
- Recognition hanya untuk siswa aktif yang terdaftar dan memiliki status legal/consent valid pada saat pengenalan.
- `unknown/inconclusive/spoof` tidak dikaitkan dengan identitas; jangan otomatis menghukum siswa.
- Identitas yang sudah diverifikasi satu kali pada sesi tidak menghasilkan baris absensi baru saat tertangkap kamera kembali.
- Admin tidak boleh mengedit jam masuk yang sudah dicatat tanpa catatan koreksi yang dapat diaudit.
- Hak akses dibatasi `school_id`, `class_id`, `device_id`, dan role.

## 7. Requirement non-fungsional (target pilot, bukan klaim)

| ID | Kategori | Target pengujian |
|---|---|---|
| NFR-01 | Latency | P95 wajah layak → keputusan UI ≤2 detik pada pilot |
| NFR-02 | False identification | Sasaran FPIR ≤0,1% di set uji; tidak diinterpretasi sebagai bukti sebelum sample memadai |
| NFR-03 | Availability | Target operasional yang disepakati sekolah; mode manual saat outage |
| NFR-04 | Performance | Uji 500 siswa, 1-2 kiosk, burst saat jam masuk |
| NFR-05 | Reliability | Tidak ada absensi ganda, uji concurrent dan replay request |
| NFR-06 | Privacy | Tidak menyimpan gambar/video wajah mentah default; retensi ketat |
| NFR-07 | Security | HTTPS, RBAC, secrets, at-rest encryption, audit, endpoint internal |
| NFR-08 | Accessibility | Keyboard navigable, labels, status tak hanya warna, target WCAG 2.2 AA |
| NFR-09 | Maintainability | Modular monolith, migration, lint, test, typed API |
| NFR-10 | Observability | Structured logs tanpa PII, tracing dengan `request_id`, health metrics |

## 8. Data minimal yang perlu dikumpulkan

Siswa: NIS, nama, kelas dan status aktif. Tahun ajar dan class enrollment untuk sejarah; wali hanya informasi yang diperlukan untuk proses consent. NISN, foto profil dan kontak tidak otomatis wajib. Wajah mentah adalah input sementara, **bukan** identitas permanen.

## 9. Kriteria rilis MVP (go/no-go)

- Skenario J-1 sampai J-4 lulus UAT.
- Consent withdrawal benar-benar menghentikan pencocokan dan memicu penghapusan sesuai retention plan.
- Unknown tidak false-accept pada test sesuai target evaluasi; ukuran sampel serta uncertainty dicatat.
- API role/class-level forbidden tests lulus.
- Kiosk tidak expose hasil sensitif di layar publik.
- Duplikasi concurrent dan request replay terblokir.
- DPIA/legal review dan prosedur fallback tersedia sebelum data anak digunakan.

## 10. Keputusan bisnis yang perlu divalidasi sekolah

Jadwal masuk/pulang, kebijakan izin dan sakit, rentang koreksi, pengelola consent, SOP jika sistem tidak mengenali siswa, masa retensi menurut kebutuhan sah, jumlah kamera, konektivitas lokasi kiosk, dan perangkat fisik. Blueprint memberikan default; bukan keputusan sepihak atas kebijakan sekolah.
