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
        <p className="mt-6 flex items-center justify-center gap-2 text-xs text-muted">
          <StatusDot tone={status.onebotConnected ? 'success' : 'muted'} />
          {status.onebotConnected ? 'NapCat 已连接' : 'NapCat 未连接'}
          <span className="text-muted">·</span>
          {status.masters} 位主人
        </p>
      }
    >
      {both ? (
        <div className="mb-5 flex justify-center">
          <Segment
            aria-label="登录方式"
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

// Centered brand + card scaffold shared by every auth state.
function Shell({ children, footer }: { children: ReactNode; footer?: ReactNode }) {
  return (
    <div className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-background p-6">
      <div
        aria-hidden
        className="pointer-events-none absolute -top-24 left-1/2 size-72 -translate-x-1/2 rounded-full bg-accent/15 blur-3xl"
      />
      <div
        aria-hidden
        className="pointer-events-none absolute -bottom-16 right-0 size-56 rounded-full bg-accent/10 blur-3xl"
      />
      <div className="relative w-full max-w-sm">
        <div className="mb-6 flex flex-col items-center gap-3 text-center">
          <img src="/brand-mark.svg" alt="" className="size-12" />
          <div>
            <h1 className="text-lg font-semibold text-foreground">讯枢</h1>
            <p className="text-sm text-muted">消息通知与归档</p>
          </div>
        </div>
        <Card variant="default">{children}</Card>
        {footer}
      </div>
    </div>
  )
}
