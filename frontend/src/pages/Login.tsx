import { useEffect, useState, type ReactNode } from 'react'
import { useNavigate } from 'react-router-dom'
import { Alert, Button, Card } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { api } from '../lib/api'
import type { AuthStatus } from '../lib/types'
import { AppIcon } from '../lib/icons'
import { Loader } from '../components/Loader'
import { StatusDot } from '../components/ui/StatusDot'
import { OtpPanel } from './login/OtpPanel'
import { PasswordPanel } from './login/PasswordPanel'

// Standalone login. Auth lives entirely in the Go backend; this screen only
// reads GET /api/auth/status to pick which methods to offer (OTP to masters
// and/or the break-glass password) and posts credentials — it never handles the
// session cookie itself.
export default function Login() {
  const navigate = useNavigate()
  const [status, setStatus] = useState<AuthStatus | null>(null)
  const [failed, setFailed] = useState(false)
  const [mode, setMode] = useState<'otp' | 'password'>('otp')

  const load = () => {
    setFailed(false)
    api.auth
      .status()
      .then((s) => {
        if (s.setupRequired) { navigate('/setup', { replace: true }); return }
        if (s.authed) {
          navigate('/', { replace: true })
          return
        }
        setStatus(s)
        setMode(s.otpAvailable ? 'otp' : 'password')
      })
      .catch(() => setFailed(true))
  }
  useEffect(load, [navigate])

  const onAuthed = () => navigate('/', { replace: true })

  if (failed) {
    return (
      <Shell>
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>无法连接服务</Alert.Title>
            <Alert.Description>请确认后端正在运行，然后重试。</Alert.Description>
          </Alert.Content>
        </Alert>
        <Button className="mt-4" fullWidth variant="secondary" onPress={load}>
          <AppIcon name="refresh" className="size-4" />
          重试
        </Button>
      </Shell>
    )
  }
  if (!status) return <Loader fullscreen label="正在加载…" />

  const both = status.otpAvailable && status.passwordAvailable
  const none = !status.otpAvailable && !status.passwordAvailable

  return (
    <Shell
      footer={
        <p className="flex flex-wrap items-center gap-2 text-xs text-muted">
          <StatusDot tone={status.onebotConnected ? 'success' : 'muted'} />
          {status.onebotConnected ? 'NapCat 已连接' : 'NapCat 未连接'}
          <span className="text-muted">·</span>
          {status.masters} 位主人
        </p>
      }
    >
      {both ? (
        <div className="mb-5 flex">
          <Segment
            aria-label="登录方式"
            className="w-full"
            selectedKey={mode}
            onSelectionChange={(key) => setMode(key as 'otp' | 'password')}
          >
            <Segment.Item id="otp">验证码登录</Segment.Item>
            <Segment.Item id="password">管理密码</Segment.Item>
          </Segment>
        </div>
      ) : null}

      {none ? (
        <Alert status="warning">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>暂无可用的登录方式</Alert.Title>
            <Alert.Description>
              {status.passwordConfigured
                ? '应急密码仅在没有主人或 NapCat 离线时可用，请稍后重试。'
                : '尚未配置应急密码，也没有可接收验证码的主人，请检查后端配置。'}
            </Alert.Description>
          </Alert.Content>
        </Alert>
      ) : mode === 'otp' ? (
        <OtpPanel onAuthed={onAuthed} masters={status.masters} />
      ) : (
        <PasswordPanel onAuthed={onAuthed} />
      )}
    </Shell>
  )
}

// Keep service context beside the compact auth form on wider screens.
function Shell({ children, footer }: { children: ReactNode; footer?: ReactNode }) {
  return (
    <main className="flex min-h-dvh items-center justify-center px-4 py-8 sm:px-6">
      <Card className="w-full max-w-3xl gap-0 overflow-hidden p-0 sm:grid sm:grid-cols-[0.85fr_1.15fr]">
        <div className="flex flex-col gap-6 bg-surface-secondary p-6 sm:p-8">
          <div className="flex items-center gap-3">
            <img src="/brand-mark.svg" alt="" className="size-10" />
            <div>
              <p className="text-lg font-semibold text-foreground">讯枢</p>
              <p className="text-xs text-muted">消息通知与归档</p>
            </div>
          </div>
          <div className="hidden sm:block">
            <p className="text-xl font-semibold leading-snug text-foreground">让重要消息，<br />及时抵达。</p>
            <ul className="mt-5 space-y-3 text-sm text-muted">
              <li className="flex items-center gap-2.5"><AppIcon name="connection" className="size-4 shrink-0" />连接信息源与群聊</li>
              <li className="flex items-center gap-2.5"><AppIcon name="intelligence" className="size-4 shrink-0" />按规则筛选重要消息</li>
              <li className="flex items-center gap-2.5"><AppIcon name="bell" className="size-4 shrink-0" />集中管理通知与归档</li>
            </ul>
          </div>
          {footer ? <div className="sm:mt-auto">{footer}</div> : null}
        </div>
        <div className="p-6 sm:p-8">
          <div className="mb-6">
            <h1 className="text-xl font-semibold tracking-tight text-foreground">登录讯枢</h1>
            <p className="mt-1 text-sm text-muted">进入你的消息工作台</p>
          </div>
          {children}
        </div>
      </Card>
    </main>
  )
}
