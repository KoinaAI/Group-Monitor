import assert from 'node:assert/strict'
import { chromium } from 'playwright'
import { readFile } from 'node:fs/promises'
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
 await page.getByText('安装 Skill',{exact:true}).waitFor()
 const command=page.getByRole('textbox',{name:'Skill 安装命令'})
 assert((await command.inputValue()).includes(`${base}/skills/install.py`))
 assert.match(await command.inputValue(),/--agent codex$/)
 await page.context().grantPermissions(['clipboard-read','clipboard-write'])
 await page.getByRole('button',{name:'复制安装命令',exact:true}).click()
 await page.getByText('安装命令已复制',{exact:true}).waitFor()
 assert.equal(await page.evaluate(()=>navigator.clipboard.readText()),await command.inputValue())
 await page.getByText('Claude Code',{exact:true}).click()
 assert.match(await command.inputValue(),/--agent claude$/)
 await page.getByText('~/.claude/skills/xunshu-api',{exact:true}).waitFor()
 // Also support LAN deployments without a secure-context Clipboard API.
 await page.evaluate(()=>Object.defineProperty(navigator,'clipboard',{value:undefined,configurable:true}))
 await page.getByRole('button',{name:'复制安装命令',exact:true}).click()
 await page.getByRole('status').filter({hasText:/安装命令已复制|请复制已选中的安装命令/}).waitFor()
 const [download]=await Promise.all([
  page.waitForEvent('download'),
  page.getByRole('link',{name:'下载 Skill ZIP',exact:true}).click(),
 ])
 assert.equal(download.suggestedFilename(),'xunshu-api.zip')
 const bytes=await readFile(await download.path())
 assert.equal(bytes.readUInt32LE(0),0x04034b50)
 for(const path of ['/skills/install.py','/skills/xunshu-api/SKILL.md','/skills/xunshu-api/scripts/query.py']){
  const response=await page.request.get(base+path)
  assert.equal(response.status(),200,path)
  assert(!(await response.text()).startsWith('<!doctype html>'),path)
 }
 await page.getByLabel('密钥名称',{exact:true}).fill('个人助理')
 await page.getByRole('checkbox',{name:'QQ / 校园'}).check()
 await page.getByRole('button',{name:'创建密钥',exact:true}).click()
 await page.getByText('xs_fixture_show_once',{exact:true}).waitFor()
 assert.deepEqual(created,{name:'个人助理',accountIds:['school']})
 assert(!(await command.inputValue()).includes('xs_fixture_show_once'))
 assert(!bytes.includes(Buffer.from('xs_fixture_show_once')))
 await page.reload()
 await page.getByRole('button',{name:'撤销',exact:true}).waitFor()
 assert.equal(await page.getByText('xs_fixture_show_once',{exact:true}).count(),0)
 await page.getByRole('button',{name:'撤销',exact:true}).click()
 await page.getByText('暂无 API Key',{exact:true}).waitFor()
 await page.setViewportSize({width:390,height:844})
 assert(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth))
 await page.evaluate(()=>window.scrollTo(0,0))
 await page.screenshot({path:'/tmp/xunshu-skill-install-mobile.png',fullPage:true})
 assert.deepEqual(errors,[])
 console.log('PASS install target commands, clipboard and HTTP fallback, real ZIP download/static files, key scopes, one-time secret, reload redaction, revoke, mobile layout')
}finally{await browser.close()}
