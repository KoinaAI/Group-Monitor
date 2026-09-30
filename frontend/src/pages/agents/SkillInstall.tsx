import { useRef, useState } from 'react'
import { Button } from '@heroui/react'
import { Segment } from '@heroui-pro/react'
import { SectionCard } from '../../components/ui/SectionCard'

const targets = {
  codex: { label: 'Codex', directory: '~/.codex/skills/xunshu-api' },
  claude: { label: 'Claude Code', directory: '~/.claude/skills/xunshu-api' },
} as const

// Quote even trusted instance URLs: a deployment hostname may contain shell
// metacharacters after decoding, and copied commands must stay literal.
function shellQuote(value: string) { return `'${value.replaceAll("'", "'\\''")}'` }

export function SkillInstall() {
  const [target, setTarget] = useState<keyof typeof targets>('codex')
  const [feedback, setFeedback] = useState('')
  const commandRef = useRef<HTMLTextAreaElement>(null)
  const installerURL = new URL('/skills/install.py', location.origin).href
  const command = `curl -fSL ${shellQuote(installerURL)} -o xunshu-install.py &&\npython3 xunshu-install.py --agent ${target}`
  const copy = async () => {
    setFeedback('')
    try {
      if (!navigator.clipboard?.writeText) throw new Error('clipboard unavailable')
      await navigator.clipboard.writeText(command)
      setFeedback('安装命令已复制')
    } catch {
      // Self-hosted HTTP pages can lack the Clipboard API. Keep the command
      // selectable and try the user-initiated legacy copy path there.
      commandRef.current?.focus()
      commandRef.current?.select()
      try {
        if (document.execCommand('copy')) { setFeedback('安装命令已复制'); return }
      } catch { /* Leave the selected command ready for manual copying. */ }
      setFeedback('请复制已选中的安装命令')
    }
  }
  return <SectionCard title="安装 Skill" description="让 Agent 直接通过 API 查询讯枢，无需配置 MCP。">
    <div className="flex min-w-0 flex-col gap-4">
      <Segment aria-label="安装到" className="self-start" selectedKey={target} onSelectionChange={(key) => {
        if (key === 'codex' || key === 'claude') { setTarget(key); setFeedback('') }
      }}>
        {Object.entries(targets).map(([id, item]) => <Segment.Item key={id} id={id}>{item.label}</Segment.Item>)}
      </Segment>
      <p className="text-sm text-muted">在运行 Agent 的电脑上执行以下命令，需要 Python 3 和 curl。安装到 <code className="break-all text-foreground">{targets[target].directory}</code>。</p>
      <textarea ref={commandRef} aria-label="Skill 安装命令" readOnly spellCheck={false} value={command} rows={5} className="h-44 w-full resize-none rounded-lg bg-surface-secondary p-3 font-mono text-xs leading-6 outline-offset-2 sm:h-28" />
      <div className="flex flex-wrap items-center gap-3">
        <Button onPress={copy}>复制安装命令</Button>
        <a href="/skills/xunshu-api.zip" download="xunshu-api.zip" className="text-sm text-accent hover:underline">下载 Skill ZIP</a>
        <a href="/skills/xunshu-api/SKILL.md" target="_blank" rel="noreferrer" className="text-sm text-muted hover:underline">查看 Skill</a>
      </div>
      {feedback && <p role="status" className="text-sm text-muted">{feedback}</p>}
      <p className="text-sm text-muted">安装后重新打开 Agent，并在其运行环境设置 <code>XUNSHU_URL</code> 为 <code className="break-all">{location.origin}</code>，<code>XUNSHU_API_KEY</code> 使用下方创建的密钥。安装包不包含密钥。</p>
      <details className="text-sm">
        <summary className="cursor-pointer text-muted">手动安装与更新</summary>
        <p className="mt-2 leading-relaxed text-muted">也可下载 ZIP，将其中的 <code>xunshu-api</code> 文件夹放入 Agent 的 skills 目录。其他支持 SKILL.md 的 Agent 可按其安装方式导入。已有安装不会自动覆盖；更新时先将原文件夹改名备份，再执行安装命令。</p>
      </details>
    </div>
  </SectionCard>
}
