import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser=await chromium.launch({headless:true}),page=await browser.newPage()
const errors=[];page.on('pageerror',e=>errors.push(e.message))
let keys=[],created
await page.route('**/api/**',async route=>{
 const req=route.request(),path=new URL(req.url()).pathname;let body={}
 if(path==='/api/auth/status')body={authed:true}
 else if(path==='/api/sources')body=[{id:'qq',name:'QQ',kind:'napcat',accounts:[{id:'school',name:'校园'}]}]
 else if(path==='/api/agent-keys'){
  if(req.method()==='POST'){created=req.postDataJSON();const key={...created,id:'key',createdAt:Date.now()};keys=[key];body={keys,key,apiKey:'xs_fixture_show_once'}}else body=keys
 }else if(path==='/api/agent-keys/revoke'){assert.equal(req.postDataJSON().id,'key');keys=[];body=keys}
 else if(path==='/api/events')return route.fulfill({contentType:'text/event-stream',body:': hi\n\n'})
 else if(['/api/logs','/api/escalations'].includes(path))body=[]
 else if(path==='/api/status')body={account:{}}
 else throw Error(path)
 await route.fulfill({contentType:'application/json',body:JSON.stringify(body)})
})
try{
 const base=process.env.SHOT_BASE??'http://127.0.0.1:15178'
 await page.goto(base+'/agents')
 await page.getByLabel('密钥名称',{exact:true}).fill('个人助理')
 await page.getByRole('checkbox',{name:'QQ / 校园'}).check()
 await page.getByRole('button',{name:'创建密钥',exact:true}).click()
 await page.getByText('xs_fixture_show_once',{exact:true}).waitFor()
 assert.deepEqual(created,{name:'个人助理',accountIds:['school']})
 await page.reload()
 await page.getByRole('button',{name:'撤销',exact:true}).waitFor()
 assert.equal(await page.getByText('xs_fixture_show_once',{exact:true}).count(),0)
 await page.getByRole('button',{name:'撤销',exact:true}).click()
 await page.getByText('暂无 API Key',{exact:true}).waitFor()
 await page.setViewportSize({width:390,height:844})
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
 assert.deepEqual(errors,[])
 console.log('PASS key scopes, one-time secret, reload redaction, revoke, mobile layout')
}finally{await browser.close()}
