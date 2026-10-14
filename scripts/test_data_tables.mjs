import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'

const requireWeb = createRequire(resolve('apps/web/package.json'))
const { parse, compileScript, registerTS } = requireWeb('@vue/compiler-sfc')
registerTS(() => requireWeb('typescript'))
const { build } = requireWeb('esbuild')
const { computed, reactive, nextTick, createSSRApp, h } = requireWeb('vue')
const { renderToString } = requireWeb('@vue/server-renderer')
const directory = resolve('.tools/data-table-regression')
mkdirSync(directory, { recursive: true })
writeFileSync(directory + '/package.json', JSON.stringify({ type: 'module' }))
try { symlinkSync(resolve('apps/web/node_modules'), directory + '/node_modules', 'dir') }
catch (error) { if (error.code !== 'EEXIST') throw error }
const web = resolve('apps/web/app')
writeFileSync(directory + '/entry.ts', `
export { default as Actions } from '${web}/components/TableActions.vue'
export { default as DataTable } from '${web}/components/DataTable.vue'
export * from '${web}/components/ui/table'
export * from '${web}/components/ui/tooltip'
export * from '${web}/components/ui/dropdown-menu'
export { Button } from '${web}/components/ui/button'
`)
await build({
  entryPoints: [directory + '/entry.ts'], outfile: directory + '/render.mjs',
  bundle: true, platform: 'node', format: 'esm', packages: 'external',
  alias: { '@': web },
  plugins: [{ name: 'production-vue', setup(build) {
    build.onLoad({ filter: /\.vue$/ }, ({ path }) => {
      const { descriptor } = parse(readFileSync(path, 'utf8'), { filename: path })
      const script = compileScript(descriptor, { id: path, inlineTemplate: true, templateOptions: { ssr: true } })
      return { contents: `import { computed } from 'vue';\n${script.content}`, loader: 'ts' }
    })
  } }],
})
const components = await import(pathToFileURL(directory + '/render.mjs').href)
async function render(component, props, slots) {
  const app = createSSRApp({ render: () => h(component, props, slots) })
  for (const [name, component] of Object.entries(components)) app.component(name, component)
  app.component('NuxtLink', { props: ['to'], setup: (props, { slots, attrs }) => () => h('a', { ...attrs, href: props.to }, slots.default?.()) })
  return renderToString(app)
}
async function setupComponent(name, props) {
  const path = `${web}/components/${name}.vue`
  const { descriptor } = parse(readFileSync(path, 'utf8'), { filename: path })
  const script = compileScript(descriptor, { id: name })
  const output = `${directory}/${name}.ts`
  writeFileSync(output, script.content)
  globalThis.computed = computed
  return (await import(pathToFileURL(output).href)).default.setup(props, { expose() {} })
}
const actions = [
  { label: 'Detail', icon: 'view', to: '/admin/students/example' },
  { label: 'Edit', icon: 'edit' },
  { label: 'Cabut', icon: 'revoke', destructive: true },
]

test('one or two visible actions render native icon controls with accessible names; three use a menu', async () => {
  for (const count of [1, 2, 3]) {
    const html = await render(components.Actions, { label: 'Siswa uji', actions: actions.slice(0, count) })
    if (count < 3) {
      assert.match(html, /aria-label="Detail: Siswa uji"/)
      assert.match(html, /href="\/admin\/students\/example"/)
      assert.doesNotMatch(html, /aria-haspopup="menu"/)
      if (count === 2) assert.match(html, /aria-label="Edit: Siswa uji"/)
    } else assert.match(html, /aria-haspopup="menu"/)
  }
  const hidden = await render(components.Actions, { label: 'Siswa uji', actions: actions.map((action, i) => ({ ...action, hidden: i > 0 })) })
  assert.doesNotMatch(hidden, /aria-haspopup="menu"/)
  assert.doesNotMatch(hidden, /aria-label="Edit:/)
})

test('disabled actions do not navigate or invoke handlers, and hidden actions determine the menu threshold', async () => {
  let calls = 0
  const props = reactive({ label: 'Siswa', disabled: true, actions: [{ ...actions[0], onSelect: () => calls++ }, { ...actions[1], hidden: true }] })
  const component = await setupComponent('TableActions', props)
  assert.equal(component.actions.value.length, 1)
  component.select(props.actions[0]); assert.equal(calls, 0)
  const html = await render(components.Actions, props)
  assert.doesNotMatch(html, /href=/)
  assert.match(html, /disabled/)
  props.disabled = false
  props.actions[0].disabled = true
  component.select(props.actions[0]); assert.equal(calls, 0)
  props.actions[0].disabled = false
  component.select(props.actions[0]); assert.equal(calls, 1)
  props.actions[1].hidden = false
  assert.equal(component.actions.value.length, 2)
})

test('table follows new server pages and visible columns without filtering or slicing the supplied rows', async () => {
  const props = reactive({ caption: 'Daftar siswa', rowKey: 'id', data: Array.from({ length: 20 }, (_, i) => ({ id: String(i), name: 'Siswa ' + i })), columns: [{ id: 'name', label: 'Nama' }, { id: 'actions', label: 'Tindakan', hidden: true }] })
  const component = await setupComponent('DataTable', props)
  assert.equal(component.table.getRowModel().rows.length, 20)
  assert.equal(component.visibleColumns.value.length, 1)
  props.data = [{ id: 'next-page', name: 'Siswa halaman berikutnya' }]
  props.columns[1].hidden = false
  await nextTick()
  assert.equal(component.table.getRowModel().rows.length, 1)
  assert.equal(component.table.getRowModel().rows[0].id, 'next-page')
  assert.equal(component.table.getRowModel().rows[0].original.name, 'Siswa halaman berikutnya')
  assert.equal(component.visibleColumns.value.length, 2)
  const html = await render(components.DataTable, props, { 'cell-name': ({ item }) => item.name })
  assert.match(html, /Siswa halaman berikutnya/)
  assert.match(html, /scope="col"/)
  const empty = await render(components.DataTable, { ...props, data: [], columns: props.columns.slice(0, 1) }, { empty: () => 'Tidak ada hasil filter' })
  assert.match(empty, /colspan="1"/)
  assert.match(empty, /Tidak ada hasil filter/)
})
process.on('exit', () => rmSync(directory, { recursive: true, force: true }))
