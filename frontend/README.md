# 讯枢前端

React 19 + TypeScript + Vite + Tailwind CSS 4，使用 HeroUI 3 与 HeroUI Pro。保持独立 SPA；生产环境必须将静态资源与 `/api` 部署在同一源，确保登录 Cookie 与实时事件流正常工作。

## 本地运行

```sh
npm ci
# 新环境按项目授权安装 Pro；不要把访问密钥写入源码或提交记录。
HEROUI_KEY=your-key npx -y hpsetup@latest react --auto
npm run dev
```

Vite 默认监听 5173，将 `/api` 转发到 `127.0.0.1:8787`。现有安装已经包含 Pro 时不需要重新运行 setup，也不需要迁移到 Next.js。

## 页面组织

- 工作台：总览、群组、通知归档、运行日志。
- 通知与接收人：ntfy / Bark 目标、QQ 主人。
- 处理策略：聚合规则、智能模型与附件阅读。
- 连接与数据：信息源、Agent 接入、存储备份。

侧栏展示一级入口，配置子页使用顶部导航。原有路由仍可直接访问。`⌘K` / `Ctrl+K` 打开 Pro Command 页面搜索；手机使用 Pro Sidebar 抽屉。

布局使用 Pro AppLayout、Navbar、Sidebar、KPI / KPIGroup、Widget、ListView、ActionBar、Segment、Stepper、NativeSelect、CellSwitch、CheckboxButtonGroup、ItemCard、CodeBlock 与 EmptyState。API 请求和账号范围由 `src/lib/api.ts`、`src/lib/accounts.ts` 统一管理。

表单采用随屏幕收缩的字段网格；通知目标与模型配置在宽屏并排。数量和范围放在工具栏中；提示、空状态和面板高度跟随内容，避免少量配置占满整页。归档采用列表与详情布局，手机上收起为单列，列表与详情依次浏览。

## 验证

```sh
npm run lint
npm run build
npm run test:skills
# 另一个终端先启动开发服务器：
npm run dev -- --host 127.0.0.1 --port 15178
# 首次运行时安装 Chromium：
npx playwright install chromium
SHOT_BASE=http://127.0.0.1:15178 npm run test:ui
```

UI 测试拦截全部 API，不发送真实通知、不写入实际配置。覆盖 13 个已登录路由的 1440 / 768 / 390px 布局、配置卡密度、页面搜索、移动导航、主题、初始化、信息源、ntfy / Bark、Agent 密钥与 Skill 安装、通知搜索分页及备份失败重试。截图默认输出到 `/tmp/xunshu-workspace`，可用 `SHOT_OUT` 指定目录。

GitHub Actions 执行前端构建、lint、Skill 单测及 UI 回归。Docker CI 验证镜像构建，不发布镜像；发行版发布仍由单独的 Docker publish 工作流负责。Pro 安装与 Docker 构建使用仓库 `HEROUI_KEY` secret。
