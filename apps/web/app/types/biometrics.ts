export interface FaceBox {
  x: number
  y: number
  width: number
  height: number
}
export interface FaceOverlay {
  box: FaceBox
  label: string
  accepted: boolean
  similarity?: number
  tone?: 'success' | 'warning'
}
export interface FaceStatus {
  enrolled: boolean
  enrollment_allowed: boolean
  pipeline_ready: boolean
  recognition_enabled: boolean
  block_reason: string | null
  template: null | {
    id: string
    reference_photo_version: string | null
    created_at: string
    model_sha256: string
    frame_count: number
    status: 'provisional'
  }
  evaluation: {
    status: 'not_evaluated' | 'reviewed'
    required?: string[]
    automatic_attendance_approved?: boolean
  }
}
export type KioskState =
  | 'initializing'
  | 'camera_ready'
  | 'detecting'
  | 'processing'
  | 'recognized'
  | 'already_recorded'
  | 'unknown'
  | 'inconclusive'
  | 'multiple_faces'
  | 'camera_error'
  | 'network_error'
  | 'session_inactive'
export interface Recognition {
  reference_photo?: string
  identity_continuous?: boolean
  decision: 'no_face' | 'unknown' | 'inconclusive' | 'accepted' | 'verification_required'
  recognition_enabled: boolean
  block_reason: string | null
  engine_status?: string
  reason?: string | null
  server_time?: string
  faces: { box: FaceBox; label?: string; student_id?: string; similarity?: number }[]
  attendance?: {
    outcome:
      | 'recorded'
      | 'already_recorded'
      | 'checked_out'
      | 'already_checked_out'
      | 'check_in_required'
      | 'status_locked'
      | 'correction_pending'
    mode: 'check_in' | 'check_out'
    record: {
      id: string
      student_id: string
      status: string
      check_in_at: string | null
      check_out_at: string | null
    } | null
  }
}
