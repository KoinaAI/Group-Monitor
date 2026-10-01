import { useEffect, useState } from 'react'
import { Button } from '@heroui/react'
import { Page } from '../components/Page'
import { PageHeader } from '../components/PageHeader'
import { SectionCard } from '../components/ui/SectionCard'
import { InlineError } from '../components/ui/States'
import { Loader } from '../components/Loader'
import { Toggle } from '../components/ui/Toggle'
import { NumberSetting } from '../components/ui/NumberSetting'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import type { Rules as RulesConfig } from '../lib/types'
import { KeywordEditor } from './rules/KeywordEditor'
import { OverrideEditor } from './rules/OverrideEditor'

// Aggregation and escalation rules. Seeds a local draft from api.config().rules,
// tracks dirtiness by value comparison, and persists via api.rules.save.
export default function Rules() {
  const { data: config, error, loading, reload } = useApi(api.config, [])
  const [rules, setRules] = useState<RulesConfig | null>(null)
  const [saved, setSaved] = useState('')
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (!config) return
    const seed: RulesConfig = {
      ...config.rules,
      urgentKeywords: config.rules.urgentKeywords ?? [],
      senderOverrides: config.rules.senderOverrides ?? [],
    }
    setRules(seed)
    setSaved(JSON.stringify(seed))
  }, [config])

  const dirty = !!rules && JSON.stringify(rules) !== saved
  const patch = (p: Partial<RulesConfig>) => {
    setRules((r) => (r ? { ...r, ...p } : r))
    setMessage('')
    setSaveError('')
  }
  const save = async () => {
    if (!rules) return
    setSaving(true)
    setSaveError('')
    setMessage('')
    try {
      const r = await api.rules.save(rules)
      setRules(r)
      setSaved(JSON.stringify(r))
      setMessage('规则已保存')
    } catch (e) {
      setSaveError(e instanceof Error ? e.message : '保存失败，请重试')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Page>
      <PageHeader
        title="规则"
        description="消息聚合、紧急判定与发送者级别"
        actions={<Button size="sm" onPress={save} isPending={saving} isDisabled={!dirty}>保存更改</Button>}
      />
      {saveError && <p role="alert" className="mb-4 text-sm text-danger">{saveError}</p>}
      {message && <p role="status" className="mb-4 text-sm text-success">{message}</p>}
      {loading || !rules ? (
        error ? (
          <InlineError message={error} onRetry={reload} />
        ) : (
          <Loader label="正在加载配置…" />
        )
      ) : (
        <>
        <div className="mb-3 flex flex-wrap items-center gap-x-5 gap-y-1 text-xs text-muted" aria-label="当前策略摘要">
          <span className="tabular-nums"><span className="font-medium text-foreground">{rules.quietWindowSec}s</span> 聚合窗口</span>
          <span className="tabular-nums">最长保持 {rules.maxHoldSec}s</span>
          <span className="tabular-nums">{rules.urgentKeywords.length} 个紧急关键词</span>
          <span className="tabular-nums">{rules.senderOverrides.length} 个发送者覆盖</span>
        </div>
        <fieldset disabled={saving} className="grid min-w-0 items-start gap-4">
          <SectionCard title="消息聚合与升级" description="合并连续消息，优先推送紧急通知">
          <div className="grid items-start gap-5 lg:grid-cols-2 lg:gap-8">
            <div className="grid gap-3">
              <h3 className="text-sm font-medium text-foreground">静默窗口</h3>
              <div className="grid gap-3 sm:grid-cols-[max-content_max-content] sm:gap-x-6">
              <NumberSetting
                label="静默窗口（秒）"
                description="首条消息后的聚合等待时间"
                value={rules.quietWindowSec}
                onChange={(v) => patch({ quietWindowSec: v })}
                minValue={0}
                maxValue={3600}
                step={10}
              />
              <NumberSetting
                label="最大保持（秒）"
                description="超过此时长强制发送"
                value={rules.maxHoldSec}
                onChange={(v) => patch({ maxHoldSec: v })}
                minValue={0}
                maxValue={7200}
                step={30}
              />
              </div>
            </div>
            <div className="flex flex-col gap-3">
              <h3 className="text-sm font-medium text-foreground">升级判定</h3>
              <Toggle
                label="@全体成员视为紧急"
                description="收到 @全体 时立即升级并绕过静默窗口"
                isSelected={rules.atAllUrgent}
                onChange={(v) => patch({ atAllUrgent: v })}
              />
              <Toggle
                label="群主 / 管理员消息提级"
                description="群主或管理员发言时自动提高级别"
                isSelected={rules.elevateOwnerAdmin}
                onChange={(v) => patch({ elevateOwnerAdmin: v })}
              />
            </div>
          </div>
          </SectionCard>
          <div className="grid min-w-0 items-start gap-4 lg:grid-cols-2">
          <SectionCard
            title="紧急关键词"
            description="命中任一关键词即判定为紧急"
          >
            <KeywordEditor
              value={rules.urgentKeywords}
              onChange={(v) => patch({ urgentKeywords: v })}
            />
          </SectionCard>
          <SectionCard
            title="发送者级别覆盖"
            description="为特定成员设定固定的发送者级别"
          >
            <OverrideEditor
              value={rules.senderOverrides}
              onChange={(v) => patch({ senderOverrides: v })}
            />
          </SectionCard>
          </div>
        </fieldset>
        </>
      )}
    </Page>
  )
}
