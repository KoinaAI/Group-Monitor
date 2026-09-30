import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser=await chromium.launch({headless:true})
const page=await browser.newPage({viewport:{width:1100,height:900}})
const errors=[];page.on('pageerror',e=>errors.push(e.message))
let targets=[],submitted,tested=[]
await page.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname;let body={}
 if(path==='/api/auth/status')body={authed:true}
 else if(path==='/api/sources')body=[{id:'qq',name:'QQ',kind:'napcat',accounts:[{id:'school',name:'校园'}]}]
 else if(path==='/api/notifications'){
  if(req.method()==='POST'){submitted=req.postDataJSON().targets;targets=submitted.map(t=>({...t,token:'',deviceKey:''}))}
  body=targets
 }else if(path==='/api/notifications/test'){tested.push(req.postDataJSON().id);body={ok:true}}
 else if(path==='/api/events')return route.fulfill({contentType:'text/event-stream',body:': hi\n\n'})
 else if(['/api/logs','/api/escalations'].includes(path))body=[]
 else if(path==='/api/status')body={account:{}}
 else throw Error(path)
 await route.fulfill({contentType:'application/json',body:JSON.stringify(body)})
})
try{
 await page.goto((process.env.SHOT_BASE??'http://127.0.0.1:15178')+'/notifications')
 await page.getByRole('button',{name:'添加 ntfy'}).click()
 await page.getByRole('textbox',{name:'主题',exact:true}).fill('school-notices')
 await page.getByLabel('访问令牌（可选）',{exact:true}).fill('fixture-token')
 await page.getByRole('checkbox',{name:'QQ / 校园'}).check()
 await page.getByRole('button',{name:'添加 Bark'}).click()
 await page.getByLabel('设备 Key',{exact:true}).fill('fixture-device-key')
 assert(await page.getByRole('button',{name:'发送测试通知'}).first().isDisabled())
 await page.getByRole('button',{name:'保存通知目标'}).click()
 await page.getByRole('status').filter({hasText:'通知目标已保存'}).waitFor()
 assert.equal(submitted[0].token,'fixture-token');assert.deepEqual(submitted[0].accountIds,['school']);assert.equal(submitted[1].deviceKey,'fixture-device-key')
 assert.equal(await page.getByLabel('设备 Key',{exact:true}).inputValue(),'')
 await page.getByRole('button',{name:'发送测试通知'}).nth(1).click()
 await page.getByRole('status').filter({hasText:'测试通知已发送'}).waitFor()
 assert.deepEqual(tested,[targets[1].id]);assert.deepEqual(errors,[])
 console.log('PASS ntfy/Bark setup, account filters, unsaved test guard, redaction, explicit test target')
}finally{await browser.close()}
