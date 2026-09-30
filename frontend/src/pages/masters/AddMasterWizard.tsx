import { useEffect, useState } from 'react'
import {
  Alert,
  Avatar,
  Button,
  Input,
  InputOTP,
  Label,
  ListBox,
  REGEXP_ONLY_DIGITS,
  Select,
  TextField,
} from '@heroui/react'
import { Stepper } from '@heroui-pro/react'
import { AppIcon } from '../../lib/icons'
import { api, ApiError } from '../../lib/api'
import { masterKindLabel, minLevelLabel } from '../../lib/labels'
import { userAvatar } from '../../lib/qlogo'
import type { Master, MasterKind, MinLevel } from '../../lib/types'

const LEVELS: MinLevel[] = [0, 1, 2, 3]
const KINDS: MasterKind[] = ['full', 'notify']
type Phase = 'input' | 'review' | 'otp'

// Secure add-master flow (Task 3). Binding a master is privileged, so a typed
// QQ number is not trusted on its own: (1) look it up so the operator can eyeball
// the real avatar + nickname, (2) DM a 3-minute one-time code to that QQ, (3)
// bind only once the code is read back here. The code is minted and verified
// entirely by the backend; this component just drives the three steps.
export function AddMasterWizard({
  existing,
  onBound,
}: {
  existing: Master[]
  onBound: (masters: Master[], nickname: string) => void
}) {
  const [phase, setPhase] = useState<Phase>('input')
  const [qid, setQid] = useState('')
  const [candidate, setCandidate] = useState<{ userId: number; nickname: string } | null>(null)
  const [level, setLevel] = useState<MinLevel>(1)
  const [kind, setKind] = useState<MasterKind>('full')
  const [code, setCode] = useState('')
  const [cooldown, setCooldown] = useState(0)
  const [pending, setPending] = useState(false)
  const [err, setErr] = useState<string>()
  const [info, setInfo] = useState<string>()

  useEffect(() => {
    if (cooldown <= 0) return
    const t = setInterval(() => setCooldown((c) => Math.max(0, c - 1)), 1000)
    return () => clearInterval(t)
  }, [cooldown])

  const reset = () => {
    setPhase('input')
    setQid('')
    setCandidate(null)
    setLevel(1)
    setKind('full')
    setCode('')
    setCooldown(0)
    setErr(undefined)
    setInfo(undefined)
  }

  // Step 1 → 2: resolve the QQ to a real profile for the operator to confirm.
  const lookup = async () => {
    const id = Number(qid.trim())
    if (!id || id <= 0 || pending) {
      setErr('请输入有效的 QQ 号')
      return
    }
    if (existing.some((m) => m.userId === id)) {
      setErr('该 QQ 已是主人')
      return
    }
    setPending(true)
    setErr(undefined)
    try {
      const si = await api.lookup(id)
      setCandidate({ userId: id, nickname: si.nickname || '' })
      setPhase('review')
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '查询失败')
    } finally {
      setPending(false)
    }
  }

  // Step 2 → 3: DM the 3-minute bind code to the confirmed candidate.
  const sendCode = async () => {
    if (!candidate || pending || cooldown > 0) return
    setPending(true)
    setErr(undefined)
    try {
      const r = await api.masters.verifyRequest(candidate.userId)
      setCandidate((c) => (c ? { ...c, nickname: r.nickname || c.nickname } : c))
      setCode('')
      setPhase('otp')
      setInfo(`验证码已发送至 ${r.nickname || candidate.userId}`)
      setCooldown(Math.min(r.ttlSec || 180, 300))
    } catch (e) {
      setErr(e instanceof ApiError ? e.message : '验证码发送失败')
    } finally {
      setPending(false)
    }
  }

  // Step 3: verify the code; the backend binds and returns the fresh master set.
  const verify = async (value: string) => {
    if (!candidate || pending) return
    setPending(true)
    setErr(undefined)
    try {
      const r = await api.masters.verifyConfirm(candidate.userId, value, level, kind)
      if (r.ok) {
        onBound(r.masters ?? [], candidate.nickname || String(candidate.userId))
        reset()
      } else {
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

  const levelSelect = (
    <Select
      aria-label="通知级别"
      variant="secondary"
      selectionMode="single"
      value={level}
      onChange={(v) => {
        if (v != null) setLevel(Number(v) as MinLevel)
      }}
      className="w-40"
    >
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {LEVELS.map((lv) => (
            <ListBox.Item key={lv} id={lv} textValue={minLevelLabel[lv]}>
              <Label>{minLevelLabel[lv]}</Label>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )

  const kindSelect = (
    <Select
      aria-label="主人类型"
      variant="secondary"
      selectionMode="single"
      value={kind}
      onChange={(v) => {
        if (v != null) setKind(String(v) as MasterKind)
      }}
      className="w-40"
    >
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {KINDS.map((k) => (
            <ListBox.Item key={k} id={k} textValue={masterKindLabel[k]}>
              <Label>{masterKindLabel[k]}</Label>
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )

  return (
    <div className="flex flex-col gap-4">
      <Stepper
        aria-label="主人绑定进度"
        currentStep={phase === 'input' ? 0 : phase === 'review' ? 1 : 2}
        size="sm"
      >
        {['查询', '核对', '验证'].map((title) => (
          <Stepper.Step key={title}>
            <Stepper.Indicator />
            <Stepper.Content>
              <Stepper.Title>{title}</Stepper.Title>
            </Stepper.Content>
            <Stepper.Separator />
          </Stepper.Step>
        ))}
      </Stepper>
      {phase === 'input' || !candidate ? (
        <div className="flex items-end gap-2">
          <TextField aria-label="QQ 号" value={qid} onChange={setQid} className="min-w-0 flex-1">
            <Label>QQ 号</Label>
            <Input placeholder="QQ 号码" variant="secondary" inputMode="numeric" />
          </TextField>
          <Button onPress={lookup} isPending={pending} isDisabled={!qid.trim()}>
            <AppIcon name="search" className="size-4" />
            查询
          </Button>
        </div>
      ) : phase === 'review' ? (
        <>
          <div className="flex items-center gap-3">
            <Avatar size="md" className="shrink-0">
              <Avatar.Image src={userAvatar(candidate.userId)} alt={candidate.nickname} loading="lazy" />
              <Avatar.Fallback>{(candidate.nickname || 'Q').slice(0, 1)}</Avatar.Fallback>
            </Avatar>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-foreground">
                {candidate.nickname || `用户 ${candidate.userId}`}
              </p>
              <p className="text-xs text-muted tabular-nums">{candidate.userId}</p>
            </div>
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm text-muted">通知级别</span>
            {levelSelect}
          </div>
          <div className="flex items-center justify-between gap-3">
            <span className="text-sm text-muted">主人类型</span>
            {kindSelect}
          </div>
          <p className="text-xs text-muted">
            {kind === 'notify'
              ? '仅通知：只接收推送提醒，用于向他人分享通知，不具备登录与机器人权限。'
              : '完整主人：接收提醒，并可通过验证码登录后台、拥有机器人权限。'}
          </p>
          <p className="text-xs text-muted">
            确认无误后，将向该 QQ 私信发送 3 分钟有效的验证码，对方回报后即可完成绑定。
          </p>
          <div className="flex flex-wrap gap-2">
            <Button variant="tertiary" onPress={reset} isDisabled={pending}>
              重新输入
            </Button>
            <Button className="flex-1" onPress={sendCode} isPending={pending}>
              确认此人并发送验证码
            </Button>
          </div>
        </>
      ) : (
        <>
          <div className="flex items-center gap-3">
            <Avatar size="md" className="shrink-0">
              <Avatar.Image src={userAvatar(candidate.userId)} alt={candidate.nickname} loading="lazy" />
              <Avatar.Fallback>{(candidate.nickname || 'Q').slice(0, 1)}</Avatar.Fallback>
            </Avatar>
            <div className="min-w-0 flex-1">
              <p className="truncate text-sm font-medium text-foreground">
                {candidate.nickname || `用户 ${candidate.userId}`}
              </p>
              <p className="text-xs text-muted tabular-nums">{candidate.userId}</p>
            </div>
          </div>
          <div className="flex flex-col items-center gap-3">
            <InputOTP
              autoFocus
              aria-label="绑定验证码"
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
                    className="min-w-0 flex-1"
                  />
                ))}
              </InputOTP.Group>
            </InputOTP>
            {info ? <p className="text-xs text-muted">{info}</p> : null}
          </div>
          <div className="flex items-center justify-between gap-2">
            <Button
              size="sm"
              variant="ghost"
              onPress={sendCode}
              isDisabled={cooldown > 0 || pending}
              className="tabular-nums"
            >
              {cooldown > 0 ? `重新发送 (${cooldown}s)` : '重新发送'}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              isDisabled={pending}
              onPress={() => {
                setPhase('review')
                setErr(undefined)
                setCode('')
              }}
            >
              返回
            </Button>
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
