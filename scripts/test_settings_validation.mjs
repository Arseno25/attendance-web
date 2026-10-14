import assert from 'node:assert/strict'
import {test} from 'node:test'
import {schoolSettingsSchema,imageUploadSchema} from '../apps/web/app/lib/validation/settings.ts'
test('school settings require identity and accept optional contact details',()=>{
 assert.equal(schoolSettingsSchema.safeParse({name:'Sekolah',app_name:'Absensi',address:'',phone:'',email:''}).success,true)
 assert.equal(schoolSettingsSchema.safeParse({name:' ',app_name:'Absensi',address:'',phone:'',email:''}).success,false)
 assert.equal(schoolSettingsSchema.safeParse({name:'Sekolah',app_name:'Absensi',address:'',phone:'text',email:'invalid'}).success,false)
})
test('profile and logo selection accepts JPG PNG but rejects oversized and executable files',()=>{
 assert.equal(imageUploadSchema.safeParse({file:new File(['image'],'photo.jpg',{type:'image/jpeg'})}).success,true)
 assert.equal(imageUploadSchema.safeParse({file:new File(['<svg>'],'logo.svg',{type:'image/svg+xml'})}).success,false)
 assert.equal(imageUploadSchema.safeParse({file:new File([new Uint8Array(2*1024*1024+1)],'large.png',{type:'image/png'})}).success,false)
 assert.equal(imageUploadSchema.safeParse({file:null}).success,false)
})
