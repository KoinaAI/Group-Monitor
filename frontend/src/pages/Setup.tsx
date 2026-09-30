import { newAccount } from '../lib/accounts'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Card } from '@heroui/react'
import { TextSetting } from '../components/ui/TextSetting'
import { Toggle } from '../components/ui/Toggle'
import { api } from '../lib/api'
import type { LLMConfig } from '../lib/types'
import { AccountFields } from './sources/AccountFields'

export default function Setup() {
  const navigate = useNavigate()
  const [step, setStep] = useState(0)
  const [token, setToken] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [llm, setLlm] = useState<LLMConfig>({ enabled: false, baseUrl: '', apiKey: '', model: '', timeoutSec: 60, maxTokens: 4096, temperature: 0.2 })
  const [addSource, setAddSource] = useState(false)
  const [account, setAccount] = useState(newAccount)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  useEffect(() => { api.setup.status().then((s) => { if (!s.required) navigate('/login', { replace: true }) }).catch((e) => setError(String(e))) }, [navigate])
  const next = () => {
    setError('')
    if (step === 0 && (!token.trim() || new TextEncoder().encode(password).length < 10 || new TextEncoder().encode(password).length > 72 || password !== confirm)) { setError('请填写初始化令牌，并确认两次密码一致（10–72 字节）。'); return }
    if (step === 1 && llm.enabled && (!llm.baseUrl || !llm.model)) { setError('启用模型时请填写 API 地址和模型名称。'); return }
    setStep(step + 1)
  }
  const complete = async () => {
    setBusy(true); setError('')
    try {
      await api.setup.complete({ setupToken: token.trim(), password, ...(llm.enabled ? { llm } : {}), ...(addSource ? { sources: [{ id: crypto.randomUUID(), name: 'NapCat', kind: 'napcat', accounts: [account] }] } : {}) })
      navigate('/sources', { replace: true })
    } catch (e) { setError(e instanceof Error ? e.message : '初始化失败') }
    finally { setBusy(false) }
  }
  return <div className="min-h-dvh bg-background px-5 py-12">
    <div className="mx-auto max-w-xl">
      <div className="mb-8"><img src="/brand-mark.svg" alt="" className="mb-4 size-10" /><h1 className="text-2xl font-semibold">设置讯枢</h1><p className="mt-2 text-sm text-muted">{step + 1} / 3 · {['管理密码', '智能模型', '首个信息源'][step]}</p></div>
      <Card><div className="flex flex-col gap-5">
        {step === 0 && <>
          <TextSetting label="初始化令牌" value={token} onChange={setToken} type="password" description="从服务启动日志获取；也可通过 NAP_SETUP_TOKEN 设置。" />
          <TextSetting label="管理密码" value={password} onChange={setPassword} type="password" description="设置 10–72 字节的密码，用于日常登录。" />
          <TextSetting label="再次输入密码" value={confirm} onChange={setConfirm} type="password" />
        </>}
        {step === 1 && <>
          <Toggle label="启用 LLM" description="可稍后在「智能」中配置。" isSelected={llm.enabled} onChange={(enabled) => setLlm({ ...llm, enabled })} />
          {llm.enabled && <>
            <TextSetting label="API 地址" value={llm.baseUrl} onChange={(baseUrl) => setLlm({ ...llm, baseUrl })} placeholder="https://api.example.com/v1" />
            <TextSetting label="模型名称" value={llm.model} onChange={(model) => setLlm({ ...llm, model })} />
            <TextSetting label="API Key" value={llm.apiKey} onChange={(apiKey) => setLlm({ ...llm, apiKey })} type="password" />
          </>}
        </>}
        {step === 2 && <>
          <Toggle label="添加 NapCat 账号" description="默认不连接任何信息源，稍后可逐一添加。已有配置会保留。" isSelected={addSource} onChange={setAddSource} />
          {addSource && <AccountFields account={account} onChange={setAccount} />}
        </>}
        {error && <p role="alert" className="text-sm text-danger">{error}</p>}
        <div className="flex justify-between gap-3 pt-2">
          <Button variant="secondary" isDisabled={step === 0 || busy} onPress={() => setStep(step - 1)}>上一步</Button>
          {step < 2 ? <Button onPress={next}>下一步</Button> : <Button isPending={busy} onPress={complete}>完成初始化</Button>}
        </div>
      </div></Card>
    </div>
  </div>
}
