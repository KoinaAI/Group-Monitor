import { useState } from 'react'
import { Alert, Button } from '@heroui/react'
import { Widget } from '@heroui-pro/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { EmptyState, InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { AppIcon } from '../lib/icons'
import { api, ApiError } from '../lib/api'
import { useApi } from '../lib/useApi'
import type { Master, MinLevel } from '../lib/types'
import { MasterRow } from './masters/MasterRow'
import { AddMasterWizard } from './masters/AddMasterWizard'

// Master list editor. New masters are added through a verified wizard (confirm
// the looked-up avatar/nickname → OTP DM'd to the candidate → bind), so the
// add path persists server-side on its own. Existing rows tune the notification
// gate or remove a master locally, then persist the whole set with 保存更改. A
// test push (POST /api/test-notify) confirms delivery works end to end.
export default function Masters() {
  const { data, error, loading, reload, setData } = useApi(api.masters.list, [])
  const [dirty, setDirty] = useState(false)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState<string>()
  const masters = data ?? []

  const patch = (fn: (m: Master[]) => Master[]) => {
    setData((prev) => fn(prev ?? []))
    setDirty(true)
  }
  const setLevel = (userId: number, lv: MinLevel) =>
    patch((ms) => ms.map((m) => (m.userId === userId ? { ...m, minLevel: lv } : m)))
  const remove = (userId: number) => patch((ms) => ms.filter((m) => m.userId !== userId))

  // The wizard binds server-side and returns the authoritative master set. Fold
  // it in without clobbering any unsaved level edits on masters already shown
  // (keep the local copy where a userId overlaps), and don't touch `dirty`: the
  // freshly bound master is already persisted.
  const onBound = (next: Master[], nickname: string) => {
    setData((prev) => {
      const local = new Map((prev ?? []).map((m) => [m.userId, m]))
      return next.map((m) => local.get(m.userId) ?? m)
    })
    setNotice(`已通过验证码绑定主人 ${nickname}`)
  }

  const save = async () => {
    setSaving(true)
    try {
      await api.masters.save(masters)
      setDirty(false)
    } finally {
      setSaving(false)
    }
  }

  const testNotify = async () => {
    setNotice(undefined)
    try {
      const r = await api.testNotify()
      setNotice(
        r.failed.length
          ? `已发送 ${r.sent} 位，失败 ${r.failed.length} 位`
          : `已向 ${r.sent} 位主人发送测试通知`,
      )
    } catch (e) {
      setNotice(e instanceof ApiError ? e.message : '测试推送失败')
    }
  }

  return (
    <Page>
      <PageHeader
        title="主人"
        description="接收升级通知的 QQ 账号及其通知级别"
        actions={
          <>
            <Button variant="secondary" onPress={testNotify}>
              <AppIcon name="send" className="size-4" />
              测试推送
            </Button>
            <Button onPress={save} isPending={saving} isDisabled={!dirty}>
              保存更改
            </Button>
          </>
        }
      />
      {notice ? (
        <Alert status="accent" className="mb-4">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Description>{notice}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}
      <Widget>
        <Widget.Header className="flex-wrap gap-y-1.5">
          <div className="flex min-w-0 flex-col gap-0.5">
            <Widget.Title>主人管理</Widget.Title>
            <Widget.Description className="tabular-nums">
              管理升级通知的接收账号与通知门槛 · 共 {masters.length} 位
            </Widget.Description>
          </div>
        </Widget.Header>
        <Widget.Content className="!p-0">
          <div className="grid items-start divide-y divide-separator lg:grid-cols-[minmax(0,1fr)_minmax(18rem,22rem)] lg:divide-x lg:divide-y-0">
            <SectionCard
              variant="transparent"
              className="p-4 lg:pr-6"
              title="主人列表"
              description="按账号设置通知门槛"
              footer={
                <p className="text-xs leading-relaxed text-muted">
                  完整主人可登录后台并管理机器人；仅通知账号只接收推送。
                  {dirty ? <span className="ml-1 text-warning">有未保存的更改。</span> : null}
                </p>
              }
            >
              {loading ? (
                <Loader label="正在加载…" />
              ) : error ? (
                <InlineError message={error} onRetry={reload} />
              ) : masters.length === 0 ? (
                <EmptyState
                  icon="masters"
                  title="尚未添加主人"
                  description="添加至少一位主人以接收升级通知"
                />
              ) : (
                <ul aria-label="主人列表" className="divide-y divide-border">
                  {masters.map((m) => (
                    <MasterRow
                      key={m.userId}
                      master={m}
                      onLevel={(lv) => setLevel(m.userId, lv)}
                      onRemove={() => remove(m.userId)}
                    />
                  ))}
                </ul>
              )}
            </SectionCard>
            <SectionCard
              variant="transparent"
              className="p-4 lg:pl-6"
              title="添加主人"
              description="核对身份后，通过 QQ 私信验证码完成绑定"
            >
              <AddMasterWizard existing={masters} onBound={onBound} />
            </SectionCard>
          </div>
        </Widget.Content>
      </Widget>
    </Page>
  )
}
