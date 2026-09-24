import { useEffect, useState } from 'react'
import { Alert, Button, InputOTP, REGEXP_ONLY_DIGITS } from '@heroui/react'
import { api, ApiError } from '../../lib/api'
import { AppIcon } from '../../lib/icons'
import { cn } from '../../lib/cn'

// One-time code delivered to masters over QQ. Two phases: request → verify.
export function OtpPanel({
  onAuthed,
  masters,
}: {
  onAuthed: () => void
  masters: number
}) {
  const [phase, setPhase] = useState<'idle' | 'sent'>('idle')
  const [code, setCode] = useState('')
  const [info, setInfo] = useState<string>()
  const [err, setErr] = useState<string>()
  const [pending, setPending] = useState(false)
  const [cooldown, setCooldown] = useState(0)

  useEffect(() => {
    if (cooldown <= 0) return
    const t = setInterval(() => setCooldown((c) => Math.max(0, c - 1)), 1000)
    return () => clearInterval(t)
  }, [cooldown])

  const request = async () => {
    if (pending || cooldown > 0) return
    setPending(true)
    setErr(undefined)
    try {
      const r = await api.auth.otpRequest()
      if (r.sent > 0) {
        setPhase('sent')
        setCode('')
        setInfo(`验证码已发送至 ${r.sent} 位主人`)
        setCooldown(Math.min(r.ttlSec || 60, 300))
      } else {
        setErr(r.failed?.[0] ? `发送失败：${r.failed[0]}` : '验证码发送失败，请稍后重试')
      }
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '验证码发送失败')
    } finally {
      setPending(false)
    }
  }

  const verify = async (value: string) => {
    setPending(true)
    setErr(undefined)
    try {
      const r = await api.auth.otpVerify(value)
      if (r.ok) onAuthed()
      else {
        setErr(r.reason || '验证码不正确')
        setCode('')
      }
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '验证失败，请重试')
      setCode('')
    } finally {
      setPending(false)
    }
  }

  return (
    <div className="flex flex-col gap-4">
      {phase === 'idle' ? (
        <>
          <p className="text-sm text-muted">
            将向 {masters} 位主人的 QQ 发送一次性验证码，用于登录控制台。
          </p>
          <Button fullWidth isPending={pending} onPress={request}>
            <AppIcon name="mail" className="size-4" />
            发送验证码
          </Button>
        </>
      ) : (
        <>
          <div className="flex flex-col items-center gap-3">
            <InputOTP
              autoFocus
              maxLength={6}
              variant="secondary"
              value={code}
              onChange={setCode}
              onComplete={verify}
              pattern={REGEXP_ONLY_DIGITS}
              className="w-full"
            >
              <InputOTP.Group className="w-full gap-2">
                {[0, 1, 2, 3, 4, 5].map((i) => (
                  <InputOTP.Slot
                    key={i}
                    index={i}
                    className="h-12 flex-1 rounded-xl border border-default bg-default text-base data-[active=true]:border-accent data-[active=true]:ring-2 data-[active=true]:ring-accent/30"
                  />
                ))}
              </InputOTP.Group>
            </InputOTP>
            {info ? <p className="text-xs text-muted">{info}</p> : null}
          </div>
          <div className="flex items-center justify-between text-xs">
            <button
              type="button"
              onClick={request}
              disabled={cooldown > 0 || pending}
              className={cn(
                'font-medium',
                cooldown > 0 ? 'text-muted' : 'text-accent hover:underline',
              )}
            >
              {cooldown > 0 ? `重新发送 (${cooldown}s)` : '重新发送'}
            </button>
            <button
              type="button"
              onClick={() => {
                setPhase('idle')
                setErr(undefined)
                setCode('')
              }}
              className="text-muted hover:text-foreground"
            >
              返回
            </button>
          </div>
        </>
      )}
      {err ? (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Description>{err}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}
    </div>
  )
}
