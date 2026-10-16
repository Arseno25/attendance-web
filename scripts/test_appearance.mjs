import assert from 'node:assert/strict'
import { test } from 'node:test'
import { createRequire } from 'node:module'
import { readFileSync, writeFileSync, mkdirSync, symlinkSync, rmSync } from 'node:fs'
import { resolve } from 'node:path'
import { pathToFileURL } from 'node:url'
const req=createRequire(resolve('apps/web/package.json'))
const {parse,compileScript}=req('@vue/compiler-sfc')
const {ref,reactive,computed,watch,effectScope,nextTick}=req('vue')
test('appearance accepts only supported persisted preferences',async()=>{
 const mode={preference:'system'};globalThis.useColorMode=()=>mode
 const {descriptor}=parse(readFileSync('apps/web/app/components/AppearanceSettings.vue','utf8'))
 const script=compileScript(descriptor,{id:'appearance-test'}),directory=resolve('.tools/appearance-tests')
 mkdirSync(directory,{recursive:true});symlinkSync(resolve('apps/web/node_modules'),directory+'/node_modules','dir')
 writeFileSync(directory+'/package.json',JSON.stringify({type:'module'}))
 const filename=directory+'/appearance.ts';writeFileSync(filename,script.content.replaceAll('~/lib/validation/zod',pathToFileURL(resolve('apps/web/app/lib/validation/zod.ts')).href))
 try{
  const component=(await import(pathToFileURL(filename).href)).default
  const page=component.setup({}, {expose(){}})
  for(const value of ['light','dark','system']){page.selectTheme(value);assert.equal(mode.preference,value)}
  page.selectTheme('unknown');assert.equal(mode.preference,'system')
 }finally{rmSync(directory,{recursive:true,force:true})}
})


test('school opens first only for authorized managers; appearance stays available to other users',async()=>{
 const directory=resolve('.tools/settings-access-tests')
 mkdirSync(directory,{recursive:true});symlinkSync(resolve('apps/web/node_modules'),directory+'/node_modules','dir')
 writeFileSync(directory+'/package.json',JSON.stringify({type:'module'}))
 const {descriptor}=parse(readFileSync('apps/web/app/pages/admin/settings.vue','utf8'))
 const compiled=compileScript(descriptor,{id:'settings-access'})
 const filename=directory+'/settings.ts'
 writeFileSync(filename,compiled.content.replaceAll('~/lib/validation/settings',pathToFileURL(resolve('apps/web/app/lib/validation/settings.ts')).href))
 let metadata
 const calls=[]
 Object.assign(globalThis,{
  ref,reactive,computed,watch,useHead:()=>{},definePageMeta:value=>metadata=value,
  useApi:()=>async path=>{calls.push(path);return {data:{name:'School',app_name:'Attendance',address:'',phone:'',email:''}}},
  useFormValidation:()=>({errors:ref({}),validate:()=>true,clear:()=>{}}),
 })
 try{
  const component=(await import(pathToFileURL(filename).href)).default
  for(const [role,permissions,allowed] of [['operator',[],false],['super_admin',[],false],['super_admin',['school.write'],true]]){
   const scope=effectScope()
   globalThis.useAuth=()=>({session:ref({user:{role},permissions})})
   const page=scope.run(()=>component.setup({}, {expose(){}}))
   assert.deepEqual(metadata.middleware,['auth'])
   assert.equal(page.tab.value,allowed?'school':'appearance');assert.equal(calls.length,allowed?1:0)
   assert.equal(page.canManageSchool.value,allowed)
   page.tab.value='school';await nextTick()
   if(allowed){assert.deepEqual(calls,['/school/settings']);assert.equal(page.school.value.name,'School')}
   else{await page.load();await page.save();assert.equal(calls.length,0)}
   calls.length=0;scope.stop()
  }
 }finally{rmSync(directory,{recursive:true,force:true})}
})
