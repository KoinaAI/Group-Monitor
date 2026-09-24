import { useState } from 'react'
import { Alert, Button, Form, Input, Label, TextField } from '@heroui/react'
import { api, ApiError } from '../../lib/api'
import { AppIcon } from '../../lib/icons'

// Break-glass password path — the backend enables it only when there are no
// masters or NapCat is offline (GET /api/auth/status `passwordAvailable`).
export function PasswordPanel({ onAuthed }: { onAuthed: () => void }) {
  const [pw, setPw] = useState('')
  const [err, setErr] = useState<string>()
  const [pending, setPending] = useState(false)

  const submit = async () => {
    if (!pw || pending) return
    setPending(true)
    setErr(undefined)
    try {
      const r = await api.auth.password(pw)
      if (r.ok) onAuthed()
      else setErr(r.reason || '密码不正确')
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '登录失败，请重试')
    } finally {
      setPending(false)
    }
  }

  return (
    <Form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <TextField value={pw} onChange={setPw} type="password" isRequired autoFocus>
        <Label>管理密码</Label>
        <Input placeholder="输入应急管理密码" variant="secondary" />
      </TextField>
      {err ? (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Description>{err}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}
      <Button type="submit" fullWidth isPending={pending} isDisabled={!pw}>
        <AppIcon name="lock" className="size-4" />
        登录
      </Button>
      <p className="text-center text-xs text-muted">
        应急通道 · 仅在无主人或 NapCat 离线时可用
      </p>
    </Form>
  )
}
