// Fixture-only monitoring regression: never sends real messages.
import { chromium } from 'playwright'
import assert from 'node:assert/strict'
const base = process.env.SHOT_BASE ?? 'http://127.0.0.1:15178'
const browser=await chromium.launch();const page=await browser.newPage({viewport:{width:1440,height:1000}});page.setDefaultTimeout(8000)
const errors=[];page.on('pageerror',e=>errors.push(e.message));await page.addInitScript(()=>localStorage.setItem('xunshu-account','test'))
let saved;let fail=true
const status={onebotConnected:true,enabled:true,watchedGroups:1,totalGroups:2,masters:3,llmEnabled:true,jevEnabled:true,quietWindowSec:90,account:{nickname:'测试账号'},selfId:123}
const groups=[{groupId:1,groupName:'项目群',groupRemark:'产品协作',memberCount:35,watch:true},{groupId:2,groupName:'公告群',groupRemark:'',memberCount:200,watch:false}]
const notices=[{id:'n1',createdAt:Date.now(),groupId:1,group:'项目群',sourceId:'src',accountId:'test',result:{level:2,title:'周会通知',summary:'请周五带上项目材料。',time:'周五 14:00',place:'会议室',event:'项目复盘',deadline:'周四 18:00'}},{id:'n2',createdAt:Date.now()-1000,groupId:2,group:'公告群',sourceId:'src',accountId:'test',result:{level:1,title:'文档更新',summary:'开发文档已更新。'}}]
await page.route('**/api/**',async route=>{
const path=new URL(route.request().url()).pathname;let body=[]
if(path==='/api/auth/status')body={authed:true}
if(path==='/api/sources')body=[{id:'src',name:'消息源',kind:'napcat',accounts:[{id:'test',name:'测试账号',enabled:true}]}]
if(path==='/api/status')body=status
if(path==='/api/events')return route.fulfill({contentType:'text/event-stream',body:'data: '+JSON.stringify({type:'hello',data:status,ts:Date.now()})+'\n\n'})
if(path==='/api/groups')body=groups
if(path==='/api/groups/watch'){saved=route.request().postDataJSON();if(fail)return route.fulfill({status:500,contentType:'application/json',body:'{"error":"保存失败测试"}'});body={ok:true}}
if(path==='/api/logs')body=[{ts:Date.now(),level:'urgent',text:'第一条：紧急通知',group:'项目群'},{ts:Date.now()-1000,level:'info',text:'第二条：连接成功'}]
if(path==='/api/escalations')body=[{ts:Date.now(),groupId:1,group:'项目群',title:'周会通知',summary:'请周五带上项目材料。',level:2,notified:true}]
if(path==='/api/notices')body=notices
if(path==='/api/groups/history')body=[{messageId:1,messageSeq:1,userId:10,nickname:'老师',role:'member',time:1,text:'历史消息',isSelf:false}]
return route.fulfill({contentType:'application/json',body:JSON.stringify(body)})})
try{
await page.goto(base+'/groups');await page.getByRole('grid',{name:'群组列表',exact:true}).waitFor();const switches=page.getByRole('switch',{name:'监听该群'});assert.equal(await switches.count(),2)
await page.locator('[data-slot="switch"]').nth(1).click();await page.getByRole('button',{name:'保存更改',exact:true}).click();await page.getByText('保存失败测试',{exact:true}).waitFor();fail=false
await page.getByRole('button',{name:'保存更改',exact:true}).click();await page.getByRole('button',{name:'保存更改',exact:true}).waitFor({state:'hidden'});assert.equal(saved.groups[1].watch,true)
await page.locator('[data-slot="switch"]').first().click();await page.getByRole('button',{name:'放弃更改',exact:true}).click();assert.equal(await switches.first().isChecked(),true)
await page.getByLabel('搜索群组',{exact:true}).fill('公告');assert.equal(await switches.count(),1);await page.screenshot({path:'/tmp/group-monitor-groups-refactor.png',fullPage:true})
console.log('PASS groups')
await page.goto(base+'/notices');await page.getByRole('article',{name:'选中的通知'}).getByRole('heading',{name:'周会通知'}).waitFor();await page.getByRole('row',{name:/文档更新/}).click();await page.getByRole('article',{name:'选中的通知'}).getByRole('heading',{name:'文档更新'}).waitFor();await page.screenshot({path:'/tmp/group-monitor-notices-refactor.png',fullPage:true})
console.log('PASS notices')
await page.goto(base+'/logs');await page.getByText('第一条：紧急通知',{exact:true}).waitFor();await page.locator('[data-slot="segment-item"]').filter({hasText:'信息'}).click();assert.equal(await page.getByText('第一条：紧急通知',{exact:true}).count(),0);await page.getByText('第二条：连接成功',{exact:true}).waitFor()
console.log('PASS logs')
await page.goto(base+'/');await page.getByText('周会通知',{exact:true}).waitFor();await page.screenshot({path:'/tmp/group-monitor-dashboard-refactor.png',fullPage:true})
for(const path of ['/','/groups','/logs','/notices','/groups/1/history']){await page.setViewportSize({width:390,height:844});await page.goto(base+path);await page.locator('h1').waitFor();await page.waitForTimeout(250);assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),`mobile overflow ${path}`)}
assert.deepEqual(errors,[]);console.log('PASS dashboard, mobile no overflow, no browser errors')
}finally{await browser.close()}
