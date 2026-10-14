export type Role = string
export type RoleScope = 'school' | 'assigned_classes'

export interface Permission {
  key: string
  label: string
  description: string
  group: string
}

export interface RoleRecord {
  id: string
  key: Role
  name: string
  description: string
  scope: RoleScope
  is_system: boolean
  permissions: string[]
  user_count: number
}

export interface User {
  avatar_url?: string
  id: string
  school_id: string
  email: string
  full_name: string
  role: Role
  role_name: string
  is_active: boolean
  class_ids: string[]
  permissions?: string[]
}

export interface Session {
  user: User
  permissions: string[]
  school: { id: string; name: string; timezone: string; app_name?: string; logo_url?: string }
  csrf_token: string
}

export interface Result<T> {
  data: T
  request_id: string
  pagination?: { page: number; page_size: number; total: number }
}

export function displayRole(user: Pick<User, 'role' | 'role_name'>) {
  return (
    user.role_name?.trim() ||
    (
      {
        super_admin: 'Pengelola sekolah',
        admin: 'Admin',
        teacher: 'Guru',
        operator: 'Petugas',
      } as Record<string, string>
    )[user.role] ||
    'Pengguna'
  )
}
