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
import { SIDEBAR_NAV, NAV_TITLE, navSection } from '../config/nav'
import { NavLink } from 'react-router-dom'
import { QuickNavigation } from './QuickNavigation'
import { EmptyState } from './ui/States'
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
        <p className="truncate text-[11px] text-muted">消息工作空间</p>
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
  const currentSection = navSection(pathname)
  const sectionLanding: Record<string, string> = {
    '通知与接收人': '/notifications',
    '处理策略': '/rules',
    '连接与数据': '/sources',
  }
  const activeConfigRoute = currentSection ? sectionLanding[currentSection.title] : undefined
  return (
    <>
      <Sidebar.Header>
        <Brand />
      </Sidebar.Header>
      <Sidebar.Content>
        {SIDEBAR_NAV.map((section) => (
          <Sidebar.Group key={section.title}>
            <Sidebar.GroupLabel>{section.title}</Sidebar.GroupLabel>
            <Sidebar.Menu aria-label={section.title}>
              {section.items.map((item) => (
                <Sidebar.MenuItem
                  key={item.to}
                  id={item.to}
                  href={item.to}
                  textValue={item.label}
                  isCurrent={pathname === item.to || (item.to === '/groups' && pathname.startsWith('/groups/')) || (item.to === '/sources' && pathname === '/connection') || (section.title === '配置中心' && item.to === activeConfigRoute)}
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
        <div className="flex justify-center py-2 group-data-[state=collapsed]:hidden"><ThemeToggle /></div>
        <AccountFooter />
      </Sidebar.Footer>
    </>
  )
}

// Top bar: mobile menu toggle + desktop collapse trigger + page title.
function TopNav() {
  const { pathname } = useLocation()
  const section = navSection(pathname)
  const title = pathname.includes('/history') ? '聊天记录' : NAV_TITLE[pathname] ?? '连接'
  const streaming = useLive((s) => s.connected)
  return (
    <Navbar maxWidth="full">
      <Navbar.Header>
        <ProLayout.MenuToggle aria-label="打开导航" tooltip="打开导航" />
        <Sidebar.Trigger aria-label="收起或展开侧栏" />
        <span className="ml-1 hidden whitespace-nowrap text-sm font-semibold text-foreground sm:inline">
          {section?.title ?? '工作台'} <span className="mx-2 font-normal text-muted">/</span> {title}
        </span>
        <Navbar.Spacer />
        <span className="hidden items-center gap-1.5 text-xs text-muted xl:flex"><span className={`size-1.5 rounded-full ${streaming ? 'bg-success' : 'bg-warning'}`} />{streaming ? '实时同步' : '正在重连'}</span>
        <QuickNavigation />
        <AccountSelector />
      </Navbar.Header>
    </Navbar>
  )
}

export function AppLayout() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const section = navSection(pathname)
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
      className="workspace-shell"
      scrollMode="content"
      navigate={navigate}
      navbar={<TopNav />}
      toolbar={section && section.title !== '工作台' ? <nav aria-label="配置页面" className="flex gap-1 overflow-x-auto border-b border-separator px-4 py-2 sm:px-6">
        {section.items.map((item) => <NavLink key={item.to} to={item.to} className={({ isActive }) => `workspace-subnav whitespace-nowrap rounded-lg px-3 py-1.5 text-sm ${isActive ? 'workspace-subnav-active font-medium text-foreground' : 'text-muted'}`}>{item.label}</NavLink>)}
      </nav> : undefined}
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
        {needsAccount ? <div className="p-6"><EmptyState icon="connection" title="选择一个信息源账号" description="使用顶部菜单选择账号，管理它的群组、主人和连接。" action={<Link className="text-sm text-accent" to="/sources">添加或管理信息源</Link>} /></div> : <Outlet />}
      </Suspense>
    </ProLayout>
  )
}
