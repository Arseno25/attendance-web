# AI Benchmark — YuNet / SFace / MiniFASNet (2026-10-17)

Pengukuran nyata pada pipeline produksi (`services/ai`) dengan model terverifikasi
dari `services/ai/models/manifest.json`. Angka di bawah adalah hasil eksekusi lokal,
bukan estimasi. **Klaim akurasi dunia nyata tetap `NOT VALIDATED`** — lihat bagian
status.

## 1. Status

| Aspek | Status |
|---|---|
| Latensi per tahap (deteksi, embedding, matching, PAD, end-to-end) | **TERUKUR** (CPU dev ini) |
| Throughput komputasi | **TERUKUR** |
| Akurasi pada fixture 4 wajah public domain | **FIXTURE SANITY ONLY** |
| FPIR/FNIR pada dataset hold-out berskala (target: 500 identitas, ribuan probe) | **NOT VALIDATED** |
| Serangan print/replay dengan media asli (kertas/layar) | **NOT VALIDATED** |
| Persetujuan attendance otomatis | Tetap **fail-closed** (`automatic_attendance_approved: false`) |

## 2. Metode

- Perintah: `python -m app.benchmark` (kode produksi nyata: `Pipeline` YuNet/SFace
  via OpenCV DNN, `FaceMatcher` produksi, `LivenessModel` MiniFASNet via ONNX Runtime).
- Statistik memakai percentile nearest-rank (konsisten dengan `app.evaluation`).
- 150 iterasi per tahap + 5 warmup; matching 1:N dengan gallery 500 identitas × 2
  template (vektor deterministik ber-seed — biaya matching tidak bergantung konten).
- Fixture akurasi: 4 potret **public domain** (karya NASA/White House) yang diunduh
  saat runtime ke `.tools/benchmark-faces/` — **tidak disimpan di git**, bukan data
  siswa. Probe: 28 augmentasi berlabel (brightness, contrast, JPEG, noise, blur,
  skala), 4 unknown (leave-one-out), 4 aproksimasi print/replay sintetis.

## 3. Lingkungan

| Item | Nilai |
|---|---|
| CPU | AMD Ryzen 5 5500 (12 logical cores) |
| Python / OpenCV | 3.14.8 / 5.0.0 (CPU) |
| `AI_OPENCV_THREADS` | 2 |
| Memori puncak (RSS) | ≈ 391 MB |
| Muat model | 0,15 detik |
| Model | YuNet 2023mar (`8f2383e4…`), SFace 2021dec (`0ba9fbfa…`), MiniFASNet v2 (`d7b3cd9b…`) |

## 4. Hasil Latensi

| Tahap | P50 | P95 | P99 |
|---|---|---|---|
| Deteksi YuNet — 640×480 | 9,9 ms | 12,0 ms | 13,4 ms |
| Deteksi YuNet — 960×720 | 27,8 ms | 34,7 ms | 42,2 ms |
| Deteksi YuNet — 1280×720 | 39,2 ms | 44,1 ms | 50,6 ms |
| Embedding SFace (alignCrop + feature) | 9,0 ms | 10,2 ms | 10,9 ms |
| Matching 1:N — 500 identitas × 2 template | 0,060 ms | 0,068 ms | — |
| PAD MiniFASNet | 2,2 ms | 2,7 ms | 2,9 ms |
| **End-to-end `process_frame` (jalur penuh, 640×480)** | **22,6 ms** | **25,9 ms** | **29,9 ms** |

- Throughput komputasi: ≈ **44 frame/detik** (satu inferensi pada satu waktu,
  semaphore engine). Batas desain per sesi kiosk: **5 embedding/detik**
  (`min_interval` 0,2 detik; pembaruan track antar-embedding).
- Target eksperimen P95 kamera-ke-hasil < 2 detik: komponen inferensi P95 ≈ 26 ms
  (≈ 1,3% anggaran). Sisanya didominasi jaringan + UI, belum diukur end-to-end
  perangkat-kamera — **bukan klaim latensi kamera-ke-UI**.
- Deteksi bertambah mahal seiring resolusi (≈ 3× dari 640×480 ke 960×720); untuk
  kiosk CPU, resolusi tangkapan 640–960 px sudah memadai.

## 5. Hasil Akurasi — FIXTURE SANITY ONLY

Pada threshold produksi 0,8 / margin 0,1 (4 identitas potret frontal):

| Metrik | Nilai | Keterangan |
|---|---|---|
| FNIR | 0/28 | Semua probe augmentasi dikenali benar |
| FPIR | 0/4 | Semua unknown (leave-one-out) ditolak |
| Salah identifikasi | 0 | — |
| `attack_pass_rate` | **4/4 diterima** | Lihat Temuan 1 |
| Distribusi skor genuine | 0,920 – 0,992 (mean 0,972) | 28 probe |
| Distribusi skor impostor | 0,133 – 0,228 (mean 0,179) | 32 skor runner-up/unknown |
| `zero_false_accept_upper95` | **0,527** | Hanya 4 probe unknown → batas statistik tidak bermakna |

Sweep threshold (margin 0,1 tetap): FNIR = 0 dan FPIR = 0 pada rentang 0,30–0,90
pada fixture ini; `attack_pass_rate` baru turun ke 0,5 di 0,90 (tetap bukan
pertahanan yang memadai — lihat Temuan 1).

**Mengapa NOT VALIDATED:** 4 identitas potret frontal dengan pencahayaan studio
memisahkan skor sangat lebar; hal ini tidak mewakili variasi kamera kiosk
(sudut, cahaya rendah, gerak). Batas statistik atas FPIR dengan 4 probe unknown
saja mencapai 52,7% — dibutuhkan minimal ribuan probe unknown untuk klaim yang
bermakna. Evaluasi resmi tetap melalui `python -m app.evaluation` dengan dataset
hold-out berizin.

## 6. Temuan Penting

1. **Recognition menerima foto ulang.** Semua aproksimasi print/replay diterima
   `FaceMatcher` pada threshold ≤ 0,85 (`attack_pass_rate` 1,0) — wajar karena
   wajahnya memang sama. **Lapisan pengenalan saja tidak menolak print/replay**;
   pertahanan wajib berada pada PAD + capture freshness (`CAPTURE_REPLAY`) +
   challenge sekali pakai + gate liveness. Temuan ini memperkuat keharusan
   approval gate yang fail-closed.
2. **PAD MiniFASNet perlu kalibrasi kamera.** Probabilitas live pada foto statis
   bervariasi (0,51–0,99); beberapa sampel *bersih* berada di bawah ambang 0,95,
   dan pada satu identitas aproksimasi foto ulang justru mendapat skor lebih
   tinggi dari foto aslinya (0,9997 vs 0,985). Konsisten dengan catatan kode
   `liveness.py`: *"Scores need camera-specific calibration"*. Evaluasi liveness
   dengan media serangan asli (kertas, layar smartphone, video) = **NOT VALIDATED**.
3. **Threshold 0,8 / margin 0,1 bekerja pada fixture frontal** (genuine minimum
   0,920 di atas threshold), tetapi kalibrasi produksi tetap memerlukan dataset
   dari kamera target; status `provisional` pada konfigurasi sudah tepat.
4. **Matching 1:N murah** (0,06 ms untuk 500 identitas) — bottleneck nyata adalah
   deteksi pada resolusi tinggi, bukan gallery.

## 7. Referensi (Context7)

- **OpenCV Zoo — YuNet** (`face_detection_yunet`): detektor wajah ringan
  ("YuNet: A tiny millisecond-level face detector", Springer 2023); mendeteksi
  wajah ±10×10 hingga 300×300 px; demo resmi memakai `confThreshold=0.9`,
  `nmsThreshold=0.3`, input `[320, 320]` — cocok dengan konfigurasi `Pipeline`
  produksi kita. Benchmark referensi OpenCV Zoo di Jetson Nano CPU: ≈ 5,6 ms
  (`[160, 120]`, backend OpenCV DNN).
- **OpenCV Zoo — SFace** (`face_recognition_sface`): pengenalan wajah dengan
  deteksi 5 landmark dari YuNet; alur resmi `alignCrop` → `feature` → `match`
  (cosine similarity) — persis alur `Pipeline.feature`/`FaceMatcher` kita
  (embedding 128-dim ternormalisasi). Benchmark referensi di Jetson Nano CPU:
  ≈ 64,8 ms (`[150, 150]`).
- **ONNX Runtime — CPU performance/threading**: default CPU Execution Provider;
  `intra_op_num_threads=0` memakai seluruh core fisik + affinitas;
  `ExecutionMode.ORT_SEQUENTIAL`, `GraphOptimizationLevel.ORT_ENABLE_ALL`, dan
  `session.intra_op.allow_spinning=1` adalah preset cepat untuk inferensi kecil.
  `LivenessModel` kita memakai `intra=2 / inter=1` — konservatif dan stabil untuk
  beban multi-kiosk dalam container.

## 8. Cara Mengulang

```bash
cd services/ai
../../.tools/uv run python -m app.benchmark \
  --directory ../../.tools/models \
  --faces-dir ../../.tools/benchmark-faces \
  --gallery 500 --iterations 150
```

Fixture wajah diunduh saat runtime (public domain, tidak masuk git). Untuk klaim
akurasi produksi: siapkan dataset hold-out berizin (≥ ribuan probe unknown,
termasuk media serangan asli), jalankan `python -m app.evaluation <trace.json>`,
lalu ajukan hasilnya melalui approval gate (`AI_RECOGNITION_APPROVAL_PATH` +
`AI_RECOGNITION_APPROVAL_SHA256`) dengan review manusia.
