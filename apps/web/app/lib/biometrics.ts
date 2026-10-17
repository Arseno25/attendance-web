import type { FaceOverlay, KioskState, Recognition } from '../types/biometrics.ts'

export const kioskStateLabels: Record<KioskState, string> = {
  initializing: 'Menyiapkan perangkat',
  camera_ready: 'Kamera siap',
  detecting: 'Mendeteksi wajah',
  processing: 'Memproses identifikasi',
  recognized: 'Berhasil dikenali',
  already_recorded: 'Sudah tercatat',
  unknown: 'Wajah belum dikenali',
  inconclusive: 'Perlu posisi lebih jelas',
  multiple_faces: 'Lebih dari satu wajah',
  camera_error: 'Kamera bermasalah',
  network_error: 'Gangguan jaringan',
  session_inactive: 'Sesi belum aktif',
}

export function deriveKioskState(input: {
  initialized: boolean
  sessionReady: boolean
  cameraActive: boolean
  cameraError?: boolean
  networkError?: boolean
  result?: Recognition | null
}): KioskState {
  if (!input.initialized) return 'initializing'
  if (input.networkError) return 'network_error'
  if (!input.sessionReady) return 'session_inactive'
  if (input.cameraError) return 'camera_error'
  if (!input.cameraActive) return 'camera_ready'
  const result = input.result
  if (!result || result.decision === 'no_face') return 'detecting'
  if (result.engine_status === 'MULTIPLE_FACES' || result.faces.length > 1) return 'multiple_faces'
  if (result.engine_status === 'PROCESSING' || result.engine_status === 'VERIFYING')
    return 'processing'
  if (result.decision === 'accepted' && result.recognition_enabled && result.attendance) {
    return ['already_recorded', 'already_checked_out'].includes(result.attendance.outcome)
      ? 'already_recorded'
      : 'recognized'
  }
  if (result.decision === 'unknown') return 'unknown'
  return 'inconclusive'
}

export function faceOverlays(result: Recognition): FaceOverlay[] {
  const accepted =
    result.decision === 'accepted' &&
    result.recognition_enabled &&
    [
      'recorded',
      'already_recorded',
      'checked_out',
      'already_checked_out',
      'check_in_required',
      'status_locked',
      'correction_pending',
    ].includes(result.attendance?.outcome ?? '')
  const continuing =
    result.decision === 'verification_required' &&
    result.identity_continuous === true &&
    !result.recognition_enabled
  return result.faces
    .filter(({ box }) => {
      const values = [box.x, box.y, box.width, box.height]
      return (
        values.every((value) => Number.isFinite(value) && value >= 0 && value <= 1) &&
        box.width > 0 &&
        box.height > 0 &&
        box.x + box.width <= 1.000001 &&
        box.y + box.height <= 1.000001
      )
    })
    .map((face) => {
      const identified = (accepted || continuing) && !!face.student_id && !!face.label?.trim()
      return {
        box: face.box,
        label: identified ? face.label! : 'Belum dikenali',
        accepted: identified && !continuing,
        tone: continuing ? ('warning' as const) : undefined,
        similarity:
          typeof face.similarity === 'number' &&
          Number.isFinite(face.similarity) &&
          face.similarity >= -1 &&
          face.similarity <= 1
            ? face.similarity
            : undefined,
      }
    })
}

export function captureJpeg(video: HTMLVideoElement) {
  if (video.readyState < 2 || !video.videoWidth || !video.videoHeight)
    throw new Error('Kamera belum siap. Tunggu preview tampil.')
  const canvas = document.createElement('canvas')
  const scale = Math.min(1, 960 / Math.max(video.videoWidth, video.videoHeight))
  canvas.width = Math.round(video.videoWidth * scale)
  canvas.height = Math.round(video.videoHeight * scale)
  if (Math.min(canvas.width, canvas.height) < 160) throw new Error('Resolusi kamera terlalu kecil.')
  const context = canvas.getContext('2d')
  if (!context)
    throw new Error('Kamera tidak dapat mengambil foto di peramban ini. Coba peramban lain.')
  context.drawImage(video, 0, 0, canvas.width, canvas.height)
  const image = canvas.toDataURL('image/jpeg', 0.75).split(',')[1]
  canvas.width = 0
  canvas.height = 0
  if (!image || image.length > 384 * 1024)
    throw new Error(
      'Foto terlalu besar. Gunakan pengaturan kamera yang lebih rendah dan coba lagi.',
    )
  return image
}

export function attendanceFeedback(attendance: NonNullable<Recognition['attendance']>) {
  const blocked: Record<string, string> = {
    check_in_required: 'Jam masuk belum tercatat. Hubungi petugas sebelum mencatat pulang.',
    status_locked: 'Status siswa sudah diatur petugas. Hubungi petugas untuk perubahan.',
    correction_pending: 'Kehadiran sedang ditinjau petugas. Tunggu koreksi selesai.',
  }
  const warning = blocked[attendance.outcome]
  if (warning)
    return {
      saved: false,
      status: 'Perlu bantuan petugas',
      tone: 'warning' as const,
      message: warning,
    }
  const departed = !!attendance.record?.check_out_at
  const late = attendance.record?.status === 'late'
  return {
    saved: !!attendance.record,
    status: departed ? 'Sudah pulang' : late ? 'Terlambat' : 'Hadir',
    tone: late && !departed ? ('warning' as const) : ('success' as const),
    message:
      attendance.outcome === 'checked_out'
        ? 'Jam pulang tersimpan.'
        : attendance.outcome === 'already_checked_out'
          ? 'Jam pulang sudah tercatat.'
          : attendance.outcome === 'recorded'
            ? 'Kehadiran tersimpan.'
            : 'Kehadiran sudah tercatat.',
  }
}
