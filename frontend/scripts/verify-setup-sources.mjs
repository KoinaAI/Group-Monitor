// Run against a Vite dev server. All API requests are fixtures; no external messages.
import assert from 'node:assert/strict'
import { chromium } from 'playwright'
const browser=await chromium.launch({headless:true})
const page=await browser.newPage({viewport:{width:1280,height:1000}})
const errors=[]; page.on('pageerror',e=>errors.push(e.message))
let initialized=false, setup, sources=[]
const requests=[]
await page.route('**/api/**', async route=>{
 const request=route.request(),url=new URL(request.url()),path=url.pathname; requests.push(url)
 let body={}
 if(path==='/api/auth/status') body={authed:initialized,setupRequired:!initialized,passwordAvailable:true}
 else if(path==='/api/setup/status') body={required:!initialized,tokenRequired:!initialized}
 else if(path==='/api/setup'){setup=request.postDataJSON();initialized=true;body={ok:true}}
 else if(path==='/api/sources'){
  if(request.method()==='POST') sources=request.postDataJSON().sources.map(s=>({...s,accounts:s.accounts.map(a=>({...a,onebot:{...a.onebot,token:''}}))}))
  body=sources
 } else if(path==='/api/sources/status') body=sources.flatMap(s=>s.accounts.map(a=>({sourceId:s.id,accountId:a.id,connected:false,selfId:0})))
 else if(path==='/api/status') body={account:{},enabled:true}
 else if(path==='/api/events') return route.fulfill({contentType:'text/event-stream',body:': ready\n\n'})
 else if(['/api/logs','/api/escalations','/api/groups'].includes(path)) body=[]
 else if(path==='/api/config') body={sources,rules:{},llm:{},jev:{}}
 else throw Error(`Unexpected API: ${path}`)
 await route.fulfill({contentType:'application/json',body:JSON.stringify(body)})
})
try {
 const base=process.env.SHOT_BASE??'http://127.0.0.1:15178'
 await page.goto(base)
 await page.getByRole('heading',{name:'设置讯枢'}).waitFor()
 await page.getByLabel('初始化令牌',{exact:true}).fill('fixture-token')
 await page.getByLabel('管理密码',{exact:true}).fill('fixture-password')
 await page.getByLabel('再次输入密码',{exact:true}).fill('fixture-password')
 await page.getByRole('button',{name:'下一步'}).click()
 await page.getByRole('button',{name:'下一步'}).click()
 await page.getByRole('button',{name:'完成初始化'}).click()
 await page.getByRole('heading',{name:'尚未添加信息源'}).waitFor()
 assert.equal(setup.password,'fixture-password');assert.equal(setup.sources,undefined)
 await page.getByRole('button',{name:'添加 NapCat 信息源'}).click()
 await page.getByRole('button',{name:'添加账号',exact:true}).click()
 await page.getByLabel('账号名称',{exact:true}).fill('校园账号')
 await page.getByLabel('OneBot HTTP 地址',{exact:true}).fill('http://localhost:3100')
 await page.getByLabel('OneBot WebSocket 地址',{exact:true}).fill('ws://localhost:3101')
 await page.getByLabel('访问令牌',{exact:true}).fill('fixture-napcat-token')
 await page.getByRole('button',{name:'保存信息源'}).click()
 await page.getByRole('status').filter({hasText:'信息源已保存'}).waitFor()
 assert.equal(sources.length,1);assert.equal(sources[0].accounts.length,1)
 assert.equal(await page.getByLabel('访问令牌',{exact:true}).inputValue(),'')
 await page.reload()
 await page.getByRole('button',{name:'管理此账号'}).click()
 await page.waitForURL(base+'/')
 await page.goto(base+'/groups')
 await page.getByRole('heading',{name:'群组',exact:true}).waitFor()
 assert(requests.some(u=>u.pathname==='/api/groups'&&u.searchParams.get('accountId')===sources[0].accounts[0].id))
 await page.setViewportSize({width:390,height:844})
 await page.goto(base+'/sources')
 await page.getByLabel('账号名称',{exact:true}).waitFor()
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
 await page.screenshot({path:'/tmp/xunshu-sources-mobile.png',fullPage:true})
 assert.deepEqual(errors,[])
 console.log('PASS setup with no source, create account, redact token, account-scoped navigation, mobile layout')
} finally {await browser.close()}
