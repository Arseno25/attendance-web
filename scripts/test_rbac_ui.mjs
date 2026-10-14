import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'

const read = (path) => readFileSync(path, 'utf8')
const roles = read('apps/web/app/pages/admin/roles.vue')
const users = read('apps/web/app/pages/admin/users.vue')
const sidebar = read('apps/web/app/components/AppSidebar.vue')
const authTypes = read('apps/web/app/types/auth.ts')
const academics = read('apps/web/app/pages/admin/academics.vue')
const validation = read('apps/web/app/lib/validation/auth.ts')

test('role page uses live catalog and role APIs with superadmin-only routing', () => {
  assert.match(roles, /middleware: \['auth', 'super-admin'\]/)
  assert.match(roles, /request<Result<RoleRecord\[]>\>\('\/roles'\)/)
  assert.match(roles, /request<Result<Permission\[]>\>\('\/permissions'\)/)
  assert.match(
    roles,
    /request\(editing\.value \? `\/roles\/\$\{editing\.value\}` : '\/roles'/,
  )
  assert.match(roles, /request\(`\/roles\/\$\{deleting\.value\.id\}`/)
  assert.match(roles, /mandatory = \['users\.read', 'users\.write', 'roles\.manage'\]/)
  assert.match(roles, /saving\.value \|\| !validate\(\)/)
  assert.match(roles, /catch \(error\) \{\s*message\.value = apiMessage\(error\)/)
  assert.ok(sidebar.includes("title: 'Peran & akses',"))
  assert.ok(sidebar.includes("url: '/admin/roles'"))
  assert.match(sidebar, /user\.role === 'super_admin'/)
})

test('user editor selects database roles and has no per-user grants contract', () => {
  assert.match(users, /request<Result<RoleRecord\[]>>\('\/roles'\)/)
  assert.match(users, /v-for="role in roles"/)
  assert.match(users, /role: form\.role/)
  assert.doesNotMatch(users, /grants/)
  assert.doesNotMatch(validation, /grants|grantIds|const roles =/)
  assert.doesNotMatch(authTypes, /grants:/)
  assert.match(authTypes, /role_name: string/)
  assert.match(authTypes, /displayRole/)
})

test('academic assignment UI posts user_id to generic class assignment endpoints', () => {
  assert.match(academics, /assignments: '\/class-assignments'/)
  assert.match(academics, /options<ClassAssignmentUser>\('\/class-assignment-users'\)/)
  assert.match(academics, /\{ user_id: form\.user_id, class_id: form\.class_id \}/)
  assert.match(academics, /form\.user_id/)
  assert.doesNotMatch(academics, /teacher_id|teacher_name|teacher-options|teacher-assignments/)
})
