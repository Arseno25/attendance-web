import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { resolve } from 'node:path'
import { loginSchema, profileSchema, userSchema } from '../apps/web/app/lib/validation/auth.ts'
import { academicYearSchema, assignmentSchema, newClassSchema, newStudentSchema, placementSchema, studentFilterSchema, importSchema, importCommitSchema } from '../apps/web/app/lib/validation/master.ts'
import { scheduleSchema, newScheduleSchema, sessionSchema, sessionFilterSchema, manualSchema, correctionSchema, decisionSchema, rosterFilterSchema } from '../apps/web/app/lib/validation/attendance.ts'
import { reportSchema, auditSchema } from '../apps/web/app/lib/validation/reports.ts'
import { roleSchema } from '../apps/web/app/lib/validation/roles.ts'
import { pairingSchema, reasonSchema, enrollmentSchema } from '../apps/web/app/lib/validation/biometrics-devices.ts'
import { useFormValidation } from '../apps/web/app/composables/useFormValidation.ts'
const { ref, nextTick, effectScope } = createRequire(resolve('apps/web/package.json'))('vue')
const id = '11111111-1111-4111-8111-111111111111'
const valid = (schema, value) => assert.equal(schema.safeParse(value).success, true)
const invalid = (schema, value, field) => {
  const result = schema.safeParse(value)
  assert.equal(result.success, false)
  if (field) assert.ok(result.error.issues.some(issue => issue.path[0] === field), JSON.stringify(result.error.issues))
}
const profile = { full_name: 'Petugas', email: 'staff@example.com', current_password: '', new_password: '', confirm_password: '' }
const schedule = { name: 'Pagi', weekdays: [1, 2], checkin_start: '07:00', checkin_cutoff: '07:30', close_time: '08:00', grace_minutes: 0, is_active: true }
const report = { from: '', to: '', class_id: '', academic_year_id: '', student_id: '', session_id: '', status: '', search: '' }

test('new passwords require eight characters, uppercase, lowercase, numbers and symbols without a maximum', () => {
  valid(loginSchema, { email: ' Staff@Example.com ', password: ' ' })
  invalid(loginSchema, { email: 'invalid', password: '' }, 'email')
  valid(loginSchema, { email: profile.email, password: 'legacy password' })
  valid(loginSchema, { email: profile.email, password: 'a'.repeat(500) })
  valid(profileSchema(profile.email), profile)
  valid(profileSchema(profile.email), { ...profile, email: ' STAFF@EXAMPLE.COM ' })
  invalid(profileSchema(profile.email), { ...profile, email: 'other@example.com' }, 'current_password')
  invalid(profileSchema(profile.email), { ...profile, new_password: 'Abcd123!', confirm_password: 'Abcd123!' }, 'current_password')
  const user = { email: profile.email, full_name: 'Petugas', role: 'admin', is_active: true, grants: [] }
  for (const password of ['Abcd123!', 'Éé12345!', 'Ab12345🔒', ' Ab12!a ', 'Ab1!' + 'a'.repeat(200)]) {
    const result = profileSchema(profile.email).parse({ ...profile, current_password: 'old', new_password: password, confirm_password: password })
    assert.equal(result.new_password, password)
    valid(userSchema(false, ['admin', 'teacher']), { ...user, password })
  }
  for (const password of ['Ab12!xy', 'abcd123!', 'ABCD123!', 'Abcdefg!', 'Abcd1234', 'Abcd123 ', 'Abcd123é']) {
    invalid(profileSchema(profile.email), { ...profile, current_password: 'old', new_password: password, confirm_password: password }, 'new_password')
    invalid(userSchema(false, ['admin', 'teacher']), { ...user, password }, 'password')
  }
  invalid(profileSchema(profile.email), { ...profile, current_password: 'old', new_password: 'Abcd123!', confirm_password: 'different' }, 'confirm_password')
})

test('user roles must exist and role permissions cannot be duplicated', () => {
  const user = { email: profile.email, full_name: 'Guru', role: 'teacher', password: 'Example123!', is_active: true }
  valid(userSchema(false, ['teacher']), user)
  valid(userSchema(true, ['teacher']), { ...user, password: '' })
  invalid(userSchema(false, ['teacher']), { ...user, password: 'short' }, 'password')
  invalid(userSchema(false, ['teacher']), { ...user, role: 'unknown' }, 'role')
  invalid(userSchema(false, ['teacher']), { ...user, full_name: '   ' }, 'full_name')
  const role = { name: 'Guru', description: '', scope: 'assigned_classes', permissions: ['reports.export'] }
  valid(roleSchema, role)
  invalid(roleSchema, { ...role, permissions: ['reports.export', 'reports.export'] }, 'permissions')
})

test('academic and student forms require dates, references, names and valid statuses', () => {
  const year = { name: '2028/2029', starts_on: '2028-02-29', ends_on: '2029-02-28', is_active: true }
  valid(academicYearSchema, year)
  invalid(academicYearSchema, { ...year, starts_on: '2027-02-29' }, 'starts_on')
  invalid(academicYearSchema, { ...year, ends_on: '2028-02-28' }, 'ends_on')
  invalid(newClassSchema, { academic_year_id: '', name: 'Kelas A', code: 'A', level: '', is_active: true }, 'academic_year_id')
  valid(assignmentSchema, { class_id: id, user_id: id })
  invalid(assignmentSchema, { class_id: id, user_id: '' }, 'user_id')
  const student = { nis: '123', nisn: '', full_name: 'Siswa', class_id: id, status: 'active' }
  valid(newStudentSchema, student)
  invalid(newStudentSchema, { ...student, status: 'deleted' }, 'status')
  invalid(newStudentSchema, { ...student, nis: ' ' }, 'nis')
  invalid(placementSchema, { class_id: id, reason: '\n\t' }, 'reason')
  invalid(studentFilterSchema, { search: 'é'.repeat(51), class_id: '', status: '' }, 'search')
})

test('schedules reject duplicate or missing weekdays, invalid times, noninteger or empty grace, and closed tolerance windows', () => {
  valid(newScheduleSchema, { ...schedule, class_id: id })
  valid(scheduleSchema, { ...schedule, grace_minutes: '5' })
  valid(scheduleSchema, { ...schedule, checkout_start: '07:45' })
  valid(scheduleSchema, { ...schedule, checkout_start: '' })
  for (const checkout_start of ['07:30', '08:00', 'invalid']) invalid(scheduleSchema, { ...schedule, checkout_start }, 'checkout_start')
  for (const weekdays of [[], [1, 1], [0], [8]]) invalid(scheduleSchema, { ...schedule, weekdays }, 'weekdays')
  for (const grace_minutes of ['', -1, 121, 0.5, 'nope']) invalid(scheduleSchema, { ...schedule, grace_minutes }, 'grace_minutes')
  invalid(scheduleSchema, { ...schedule, checkin_start: '07:30' }, 'checkin_cutoff')
  invalid(scheduleSchema, { ...schedule, grace_minutes: 30 }, 'close_time')
  invalid(scheduleSchema, { ...schedule, checkin_start: '25:00' }, 'checkin_start')
})

test('attendance mode rules retain server status authority and school-local correction times', () => {
  valid(sessionFilterSchema, { date: '' })
  invalid(sessionSchema, { date: '2026-13-01', schedule_id: id }, 'date')
  const entry = { session_id: id, student_id: id, event_id: id, status: 'present', reason: 'Diverifikasi petugas' }
  valid(manualSchema, entry)
  invalid(manualSchema, { ...entry, status: 'late' }, 'status')
  invalid(manualSchema, { ...entry, reason: '  ' }, 'reason')
  invalid(correctionSchema, { ...entry, check_in_local: '' }, 'check_in_local')
  valid(correctionSchema, { ...entry, status: 'late', check_in_local: '2026-10-14T07:50:00' })
  invalid(correctionSchema, { ...entry, check_in_local: '2026-02-30T07:00' }, 'check_in_local')
  valid(correctionSchema, { ...entry, status: 'sick', check_in_local: '' })
  invalid(decisionSchema, { decision: 'pending', reason: 'Verifikasi' }, 'decision')
  invalid(rosterFilterSchema, { search: '', status: 'deleted' }, 'status')
  invalid(rosterFilterSchema, { search: 'é'.repeat(51), status: '' }, 'search')
})

test('report defaults preserve server dates while partial, reversed and overlong ranges fail', () => {
  valid(reportSchema, report)
  valid(reportSchema, { ...report, from: '2028-01-01', to: '2028-12-31' })
  invalid(reportSchema, { ...report, from: '2026-10-14' }, 'to')
  invalid(reportSchema, { ...report, from: '2026-10-14', to: '2026-10-13' }, 'to')
  invalid(reportSchema, { ...report, from: '2028-01-01', to: '2029-01-01' }, 'to')
  invalid(reportSchema, { ...report, student_id: 'invalid' }, 'student_id')
  valid(auditSchema, { from: '2026-10-14', to: '', actor_id: '', action: '', target_type: '' })
  invalid(auditSchema, { from: '2026-10-14', to: '2026-10-13', actor_id: '', action: '', target_type: '' }, 'to')
})

test('pairing and enrollment check references and encoded JPEG bounds', () => {
  valid(pairingSchema, { pairing_token: 'a'.repeat(64), session_id: id })
  invalid(pairingSchema, { pairing_token: 'g'.repeat(64), session_id: id }, 'pairing_token')
  invalid(pairingSchema, { pairing_token: 'a'.repeat(64), session_id: 'open' }, 'session_id')
  invalid(reasonSchema, { reason: ' ' }, 'reason')
  const jpeg = '/9j/'
  valid(enrollmentSchema, { frames: [jpeg, jpeg, jpeg] })
  invalid(enrollmentSchema, { frames: [jpeg, jpeg] }, 'frames')
  invalid(enrollmentSchema, { frames: Array(6).fill(jpeg) }, 'frames')
  invalid(enrollmentSchema, { frames: ['/9j/' + 'A'.repeat(384 * 1024), jpeg, jpeg] }, 'frames')
  invalid(enrollmentSchema, { frames: ['data:image/jpeg;base64,/9j/', jpeg, jpeg] }, 'frames')
})

test('CSV preview checks size and selection while server owns CSV parsing and the 500-row domain limit', () => {
  valid(importSchema, { class_id: id, csv: 'nis,nisn,full_name\n1,,Siswa' })
  invalid(importSchema, { class_id: '', csv: 'nis,nisn,full_name\n1,,Siswa' }, 'class_id')
  invalid(importSchema, { class_id: id, csv: '' }, 'csv')
  invalid(importSchema, { class_id: id, csv: 'é'.repeat(60 * 1024 + 1) }, 'csv')
  invalid(importSchema, { class_id: id, csv: ' '.repeat(120 * 1024) + 'a' }, 'csv')
  valid(importCommitSchema, { id, hash: 'a'.repeat(64) })
  invalid(importCommitSchema, { id, hash: 'invalid' }, 'hash')
})

test('form helper revalidates, clears and focuses visible fields without entering inert background forms', async () => {
  const scope = effectScope()
  const values = ref({ reason: '' })
  let focused = 0
  const oldDocument = globalThis.document
  globalThis.document = { querySelectorAll: () => [
    { closest: () => ({}), getClientRects: () => [1], focus: () => assert.fail('inert field focused') },
    { closest: () => null, getClientRects: () => [], focus: () => assert.fail('hidden field focused') },
    { closest: () => null, getClientRects: () => [1], focus: () => focused++ },
  ] }
  try {
    const form = scope.run(() => useFormValidation(() => reasonSchema, () => values.value))
    assert.deepEqual(form.errors.value, {})
    assert.equal(form.validate(), false)
    assert.ok(form.errors.value.reason)
    await nextTick(); assert.equal(focused, 1)
    values.value.reason = 'Diverifikasi'; await nextTick(); assert.deepEqual(form.errors.value, {})
    values.value.reason = ' '; await nextTick(); assert.ok(form.errors.value.reason)
    form.clear(); values.value.reason = ''; await nextTick(); assert.deepEqual(form.errors.value, {})
  }
  finally { scope.stop(); globalThis.document = oldDocument }
})
