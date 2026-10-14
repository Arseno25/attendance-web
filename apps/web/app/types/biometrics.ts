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
    created_at: string
    model_sha256: string
    frame_count: number
    status: 'provisional'
  }
  evaluation: { status: 'not_evaluated'; required: string[] }
}
export interface Recognition {
  decision: 'no_face' | 'unknown' | 'inconclusive' | 'accepted'
  recognition_enabled: boolean
  block_reason: string | null
  faces: { box: FaceBox; label?: string; student_id?: string }[]
  attendance?: { outcome: 'recorded' | 'already_recorded' }
}
