export interface AcademicYear {
  id: string
  name: string
  starts_on: string
  ends_on: string
  is_active: boolean
}
export interface SchoolClass {
  id: string
  code: string
  name: string
  level: string
  academic_year_id: string
  academic_year_name: string
  is_active: boolean
}
export interface ClassAssignmentUser {
  id: string
  name: string
  role: string
}
export interface Assignment {
  id: string
  user_id: string
  user_name: string
  class_name: string
  academic_year_name: string
  starts_at: string
  ends_at: string
  is_active: boolean
  is_current: boolean
}
export interface Placement {
  id: string
  class_id: string
  class_name: string
  academic_year_name: string
  joined_at: string
  left_at: string
  reason: string
}
export interface Student {
  id: string
  nis: string
  nisn: string | null
  full_name: string
  status: string
  class_id: string | null
  class_name: string | null
  academic_year_name: string | null
  face_enrolled?: boolean
  placements?: Placement[]
}
export const statusLabels: Record<string, string> = {
  active: 'Aktif',
  inactive: 'Nonaktif',
  graduated: 'Lulus',
  transferred: 'Pindah sekolah',
}
