import { newAccount } from '../lib/accounts'
import { uid } from '../lib/id'
import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Alert, Button, Card, Form } from '@heroui/react'
import { Stepper } from '@heroui-pro/react'
import { AppIcon } from '../lib/icons'
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
      await api.setup.complete({ setupToken: token.trim(), password, ...(llm.enabled ? { llm } : {}), ...(addSource ? { sources: [{ id: uid(), name: 'NapCat', kind: 'napcat', accounts: [account] }] } : {}) })
      navigate('/sources', { replace: true })
    } catch (e) { setError(e instanceof Error ? e.message : '初始化失败') }
    finally { setBusy(false) }
  }
  const steps = [
    { title: '管理密码', description: '验证身份，保护工作台' },
    { title: '智能模型', description: '按需开启消息分析' },
    { title: '首个信息源', description: '连接 NapCat 账号' },
  ]

  return (
    <main className="flex min-h-dvh items-center justify-center bg-background px-4 py-8 sm:px-6">
      <div className="w-full max-w-4xl">
        <header className="mb-5 flex items-center gap-3">
          <img src="/brand-mark.svg" alt="" className="size-10" />
          <div>
            <h1 className="text-xl font-semibold tracking-tight">设置讯枢</h1>
            <p className="mt-0.5 text-sm text-muted">完成基础设置，开始管理消息与通知</p>
          </div>
          <span className="ml-auto shrink-0 text-sm tabular-nums text-muted">{step + 1} / 3</span>
        </header>
        <Card className="gap-0 overflow-hidden p-0 md:grid md:grid-cols-[240px_minmax(0,1fr)]">
          <aside className="bg-surface-secondary p-5 md:p-6">
            <Stepper currentStep={step} orientation="vertical" size="md" aria-label="初始化进度">
              {steps.map((item) => (
                <Stepper.Step key={item.title}>
                  <Stepper.Indicator />
                  <Stepper.Content>
                    <Stepper.Title>{item.title}</Stepper.Title>
                    <Stepper.Description>{item.description}</Stepper.Description>
                  </Stepper.Content>
                  <Stepper.Separator />
                </Stepper.Step>
              ))}
            </Stepper>
            <p className="mt-6 hidden text-xs leading-relaxed text-muted md:block">模型和信息源均为可选项，可在进入工作台后继续配置。</p>
          </aside>
          <Form
            className="flex min-w-0 flex-col gap-5 p-5 sm:p-6"
            onSubmit={(event) => {
              event.preventDefault()
              if (step < 2) next()
              else if (!busy) void complete()
            }}
          >
            <div>
              <h2 className="text-base font-semibold">{steps[step].title}</h2>
              <p className="mt-1 text-sm text-muted">{[
                '验证初始化令牌，并设置用于登录的管理密码。',
                '连接兼容 OpenAI 的模型服务，辅助识别重要消息。',
                '添加接收消息的账号，或稍后在信息源中连接。',
              ][step]}</p>
            </div>
            {step === 0 && <>
              <TextSetting label="初始化令牌" value={token} onChange={setToken} type="password" description="从服务启动日志获取；也可通过 NAP_SETUP_TOKEN 设置。" />
              <div className="grid gap-4 sm:grid-cols-2">
                <TextSetting label="管理密码" value={password} onChange={setPassword} type="password" description="10–72 字节，用于日常登录。" />
                <TextSetting label="再次输入密码" value={confirm} onChange={setConfirm} type="password" />
              </div>
            </>}
            {step === 1 && <>
              <Toggle label="启用 LLM" description="可稍后在「智能」中配置。" isSelected={llm.enabled} onChange={(enabled) => setLlm({ ...llm, enabled })} />
              {llm.enabled && <div className="grid gap-4 sm:grid-cols-2">
                <TextSetting className="sm:col-span-2" label="API 地址" value={llm.baseUrl} onChange={(baseUrl) => setLlm({ ...llm, baseUrl })} placeholder="https://api.example.com/v1" />
                <TextSetting label="模型名称" value={llm.model} onChange={(model) => setLlm({ ...llm, model })} />
                <TextSetting label="API Key" value={llm.apiKey} onChange={(apiKey) => setLlm({ ...llm, apiKey })} type="password" />
              </div>}
            </>}
            {step === 2 && <>
              <Toggle label="添加 NapCat 账号" description="默认不连接任何信息源，稍后可逐一添加。已有配置会保留。" isSelected={addSource} onChange={setAddSource} />
              {addSource && <AccountFields account={account} onChange={setAccount} />}
            </>}
            {error && <Alert status="danger"><Alert.Indicator /><Alert.Content><Alert.Description>{error}</Alert.Description></Alert.Content></Alert>}
            <div className="flex items-center justify-between gap-3 pt-1">
              <Button variant="tertiary" isDisabled={step === 0 || busy} onPress={() => { setError(''); setStep(step - 1) }}><AppIcon name="back" className="size-4" />上一步</Button>
              {step < 2
                ? <Button type="submit">下一步<AppIcon name="chevronRight" className="size-4" /></Button>
                : <Button type="submit" isPending={busy}>完成初始化<AppIcon name="check" className="size-4" /></Button>}
            </div>
          </Form>
        </Card>
      </div>
    </main>
  )
}
