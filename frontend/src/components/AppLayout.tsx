import { Suspense, useEffect } from 'react'
import { Outlet, useLocation, useNavigate } from 'react-router-dom'
import { AppLayout as ProLayout, Navbar, Sidebar, useSidebar } from '@heroui-pro/react'
import { Button, Tooltip } from '@heroui/react'
import { AccountSelector } from './AccountSelector'
import { activeAccount } from '../lib/accounts'
import { Link } from 'react-router-dom'
import { Loader } from './Loader'
import { ThemeToggle } from './ThemeToggle'
import { AppIcon } from '../lib/icons'
import { NAV, NAV_TITLE } from '../config/nav'
import { api } from '../lib/api'
import { useApi } from '../lib/useApi'
import { connectLiveStream, useLive } from '../lib/store'

// The HeroUI Pro application shell: an AppLayout hosting a collapsible Sidebar
// (grouped nav from ../config/nav) plus a top Navbar, with the routed page in
// the main slot. AppLayout provides its own Sidebar.Provider — we must NOT add
// one. The authed area also owns the live SSE stream (opened on mount, closed
// on unmount) so every page reads from one shared zustand store.

// Brand block. The text collapses away in icon-rail mode: HeroUI hides spans
// marked data-sidebar="label" when the sidebar is collapsed to icons.
function Brand() {
  return (
    <div className="flex items-center gap-2.5 px-1.5 py-1">
      <img src="/brand-mark.svg" alt="" className="size-7 shrink-0" />
      <div
        className="min-w-0 leading-tight group-data-[state=collapsed]:hidden"
        data-sidebar="label"
      >
        <p className="truncate text-sm font-semibold text-foreground">讯枢</p>
        <p className="truncate text-xs text-muted">消息通知与归档</p>
      </div>
    </div>
  )
}

// Account + NapCat connection state, with logout. Connection is live (SSE
// store); the nickname comes from a one-shot GET /api/status.
function AccountFooter() {
  const navigate = useNavigate()
  const { collapsible, isMobile, isOpen } = useSidebar()
  const collapsed = collapsible === 'icon' && !isMobile && !isOpen
  const connected = useLive((s) => s.status?.onebotConnected ?? false)
  const selfId = useLive((s) => s.status?.selfId ?? 0)
  const { data: status } = useApi(api.status, [])
  const name = status?.account?.nickname || (selfId ? String(selfId) : activeAccount() ? 'NapCat' : '管理账户')

  const logout = () =>
    api.auth.logout().finally(() => navigate('/login', { replace: true }))

  const logoutBtn = (
    <Tooltip>
      <Button isIconOnly size="sm" variant="ghost" aria-label="退出登录" onPress={logout}>
        <AppIcon name="logout" className="size-4" />
      </Button>
      <Tooltip.Content>退出登录</Tooltip.Content>
    </Tooltip>
  )

  // Icon rail: the label collapses away, so a horizontal dot+text+button row
  // would overflow 48px. Stack a status dot above the logout button instead,
  // both centered.
  if (collapsed) {
    return (
      <div className="flex flex-col items-center gap-2 py-1">
        <span
          className={`size-2 rounded-full ${connected ? 'bg-success' : 'bg-muted'}`}
          aria-label={connected ? 'NapCat 已连接' : activeAccount() ? 'NapCat 未连接' : '密码登录'}
        />
        {logoutBtn}
      </div>
    )
  }

  return (
    <div className="flex items-center gap-2 px-1.5 py-1">
      <span
        className={`size-2 shrink-0 rounded-full ${connected ? 'bg-success' : 'bg-muted'}`}
        aria-hidden
      />
      <div className="min-w-0 flex-1 leading-tight" data-sidebar="label">
        <p className="truncate text-xs font-medium text-foreground">{name}</p>
        <p className="truncate text-[11px] text-muted">
          {connected ? 'NapCat 已连接' : activeAccount() ? 'NapCat 未连接' : '密码登录'}
        </p>
      </div>
      {logoutBtn}
    </div>
  )
}

// Header + grouped menu + footer. Rendered identically in the desktop rail and
// the mobile drawer.
function SidebarInner() {
  const { pathname } = useLocation()
  return (
    <>
      <Sidebar.Header>
        <Brand />
      </Sidebar.Header>
      <Sidebar.Content>
        {NAV.map((section) => (
          <Sidebar.Group key={section.title}>
            <Sidebar.GroupLabel>{section.title}</Sidebar.GroupLabel>
            <Sidebar.Menu aria-label={section.title}>
              {section.items.map((item) => (
                <Sidebar.MenuItem
                  key={item.to}
                  id={item.to}
                  href={item.to}
                  textValue={item.label}
                  isCurrent={pathname === item.to}
                >
                  <Sidebar.MenuIcon>
                    <AppIcon name={item.icon} className="size-4" />
                  </Sidebar.MenuIcon>
                  <Sidebar.MenuLabel>{item.label}</Sidebar.MenuLabel>
                </Sidebar.MenuItem>
              ))}
            </Sidebar.Menu>
          </Sidebar.Group>
        ))}
      </Sidebar.Content>
      <Sidebar.Footer>
        <AccountFooter />
      </Sidebar.Footer>
    </>
  )
}

// Top bar: mobile menu toggle + desktop collapse trigger + page title.
function TopNav() {
  const { pathname } = useLocation()
  const title = NAV_TITLE[pathname] ?? '讯枢'
  return (
    <Navbar maxWidth="full">
      <Navbar.Header>
        <ProLayout.MenuToggle />
        <Sidebar.Trigger />
        <span className="ml-1 hidden whitespace-nowrap text-sm font-semibold text-foreground sm:inline">
          {title}
        </span>
        <Navbar.Spacer />
        <AccountSelector />
        <ThemeToggle />
      </Navbar.Header>
    </Navbar>
  )
}

export function AppLayout() {
  const { pathname } = useLocation()
  const needsAccount = /^(\/groups|\/masters|\/connection)/.test(pathname) && !activeAccount()
  useEffect(() => {
    const stop = connectLiveStream()
    const { seedEscalations, seedLogs } = useLive.getState()
    api.escalations().then(seedEscalations).catch(() => {})
    api.logs().then(seedLogs).catch(() => {})
    return stop
  }, [])

  return (
    <ProLayout
      navbar={<TopNav />}
      sidebar={
        <>
          <Sidebar className="group">
            <SidebarInner />
          </Sidebar>
          <Sidebar.Mobile>
            <SidebarInner />
          </Sidebar.Mobile>
        </>
      }
    >
      <Suspense fallback={<Loader label="正在加载…" />}>
        {needsAccount ? <div className="p-8"><h2 className="text-lg font-semibold">选择一个信息源账号</h2><p className="mt-2 text-sm text-muted">使用顶部菜单选择账号，分别管理它的群、主人和规则。</p><Link className="mt-4 inline-block text-accent" to="/sources">添加或管理信息源</Link></div> : <Outlet />}
      </Suspense>
    </ProLayout>
  )
}
