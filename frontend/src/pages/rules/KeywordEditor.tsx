import { useState } from 'react'
import { Button, Input, Tag, TagGroup, TextField } from '@heroui/react'
import { AppIcon } from '../../lib/icons'

// Tag-style editor for the urgent-keyword list. Enter or the add button appends;
// each chip has an inline remove.
export function KeywordEditor({
  value,
  onChange,
}: {
  value: string[]
  onChange: (v: string[]) => void
}) {
  const [input, setInput] = useState('')
  const add = () => {
    const k = input.trim()
    if (!k || value.includes(k)) {
      setInput('')
      return
    }
    onChange([...value, k])
    setInput('')
  }
  return (
    <div className="flex flex-col gap-3">
      <div className="flex items-end gap-2">
        <TextField aria-label="紧急关键词" value={input} onChange={setInput} className="flex-1">
          <Input
            placeholder="输入关键词后回车或点添加"
            variant="secondary"
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                add()
              }
            }}
          />
        </TextField>
        <Button variant="secondary" onPress={add} isDisabled={!input.trim()}>
          <AppIcon name="add" className="size-4" />
          添加
        </Button>
      </div>
      {value.length ? (
        <TagGroup
          aria-label="紧急关键词"
          onRemove={(keys) => onChange(value.filter((k) => !keys.has(k)))}
        >
          <TagGroup.List>
            {value.map((k) => (
              <Tag key={k} id={k} textValue={k}>
                {k}
              </Tag>
            ))}
          </TagGroup.List>
        </TagGroup>
      ) : (
        <p className="text-xs text-muted">未设置关键词，命中即判定为紧急并绕过静默窗口。</p>
      )}
    </div>
  )
}
