export type Role = 'super_admin' | 'admin' | 'teacher' | 'operator'

export interface User {
  id: string
  school_id: string
  email: string
  full_name: string
  role: Role
  is_active: boolean
  grants: string[]
  class_ids: string[]
}

export interface Session {
  user: User
  permissions: string[]
  school: { id: string, name: string, timezone: string }
  csrf_token: string
}

export interface Result<T> {
  data: T
  request_id: string
  pagination?: { page: number, page_size: number, total: number }
}

export const roleLabels: Record<Role, string> = {
  super_admin: 'Super admin', admin: 'Admin', teacher: 'Guru', operator: 'Operator',
}
