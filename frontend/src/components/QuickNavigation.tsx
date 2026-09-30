import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button, Kbd, Tooltip } from '@heroui/react'
import { Command } from '@heroui-pro/react'
import { NAV } from '../config/nav'
import { AppIcon } from '../lib/icons'

export function QuickNavigation() {
  const [open, setOpen] = useState(false)
  const navigate = useNavigate()
  useEffect(() => {
    const onKey = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'k') {
        event.preventDefault()
        setOpen((value) => !value)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])
  return <>
    <Tooltip>
      <Button variant="ghost" size="sm" aria-label="搜索页面" onPress={() => setOpen(true)} className="gap-2 px-2 text-muted">
        <AppIcon name="search" className="size-4" />
        <span className="hidden lg:inline">搜索页面</span>
        <Kbd className="hidden lg:inline-flex">⌘ K</Kbd>
      </Button>
      <Tooltip.Content>搜索页面 · ⌘ / Ctrl K</Tooltip.Content>
    </Tooltip>
    <Command>
      <Command.Backdrop isOpen={open} onOpenChange={setOpen}>
        <Command.Container>
          <Command.Dialog aria-label="搜索页面">
            <Command.InputGroup aria-label="搜索页面">
              <Command.InputGroup.Prefix><AppIcon name="search" className="size-5" /></Command.InputGroup.Prefix>
              <Command.InputGroup.Input placeholder="搜索页面或配置…" />
              <Command.InputGroup.ClearButton aria-label="清除搜索" />
            </Command.InputGroup>
            <Command.List aria-label="页面" renderEmptyState={() => '没有匹配页面'} onAction={(key) => { setOpen(false); navigate(String(key)) }}>
              {NAV.map((section) => <Command.Group key={section.title} heading={section.title}>
                {section.items.map((item) => <Command.Item key={item.to} id={item.to} textValue={`${item.label} ${item.hint}`}>
                  <AppIcon name={item.icon} className="size-4 text-muted" />
                  <span className="flex flex-col"><span>{item.label}</span><span className="text-xs text-muted">{item.hint}</span></span>
                </Command.Item>)}
              </Command.Group>)}
            </Command.List>
            <Command.Footer className="text-xs text-muted">方向键选择 · Enter 打开 · Esc 关闭</Command.Footer>
          </Command.Dialog>
        </Command.Container>
      </Command.Backdrop>
    </Command>
  </>
}
