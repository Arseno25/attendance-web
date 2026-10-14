import assert from 'node:assert/strict'
import {test} from 'node:test'
import {createRequire} from 'node:module'
import {readFileSync,writeFileSync,mkdirSync,symlinkSync,rmSync} from 'node:fs'
import {resolve} from 'node:path'
import {pathToFileURL} from 'node:url'
import {useFormValidation} from '../apps/web/app/composables/useFormValidation.ts'
const req=createRequire(resolve('apps/web/package.json')), {parse,compileScript}=req('@vue/compiler-sfc'), {ref,computed,watch,nextTick,effectScope}=req('vue')
test('image upload validates before sending, previews locally and prevents duplicate requests',async()=>{
 let unmount,finish,calls=0,payload
 Object.assign(globalThis,{ref,computed,watch,useFormValidation,useId:()=> 'test-photo',useRuntimeConfig:()=>({public:{apiBase:'http://localhost/api/v1'}}),useApi:()=>async(_path,options)=>{calls++;payload=options;return new Promise(r=>{finish=r})},onBeforeUnmount:fn=>{unmount=fn},apiMessage:()=> 'Failed'})
 const directory=resolve('.tools/upload-tests');mkdirSync(directory,{recursive:true});symlinkSync(resolve('apps/web/node_modules'),directory+'/node_modules','dir')
 const {descriptor}=parse(readFileSync('apps/web/app/components/ImageUploadField.vue','utf8')),script=compileScript(descriptor,{id:'photo-test'}),file=directory+'/upload.ts'
 writeFileSync(file,script.content.replaceAll('~/lib/validation/settings',pathToFileURL(resolve('apps/web/app/lib/validation/settings.ts')).href))
 const module=await import(pathToFileURL(file).href),scope=effectScope(),events=[]
 const page=scope.run(()=>module.default.setup({endpoint:'/auth/profile/photo',label:'Foto profil',value:''},{expose(){},emit:(...args)=>events.push(args)}))
 try{
  page.choose({target:{files:[new File(['bad'],'bad.svg',{type:'image/svg+xml'})]}})
  await page.save();assert.equal(calls,0);assert.ok(page.errors.value.file)
  const photo=new File(['synthetic'],'test.png',{type:'image/png'})
  page.choose({target:{files:[photo]}});assert.ok(page.preview.value.startsWith('blob:'))
  const saving=page.save();await page.save();assert.equal(calls,1);assert.equal(payload.body,photo);assert.equal(payload.method,'PUT')
  finish({data:{image_url:'/auth/profile/photo?v=test'}});await saving;await nextTick()
  assert.equal(page.file.value,null);assert.equal(page.preview.value,'');assert.equal(page.image.value,'http://localhost/api/v1/auth/profile/photo?v=test');assert.ok(events.some(e=>e[0]==='updated'))
 }finally{unmount();scope.stop();rmSync(directory,{recursive:true,force:true})}
})
