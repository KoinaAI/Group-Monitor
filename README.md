# 讯枢

面向群聊与协作平台的智能消息通知工具，聚焦消息筛选、分级提醒与通知归档。当前版本通过 NapCat / OneBot v11 接入 QQ 群聊，架构支持后续扩展更多消息渠道。

## 支持渠道

| 渠道 | 状态 | 当前接入方式 |
|---|---|---|
| QQ 群聊 | 已支持 | NapCat / OneBot v11 |
|  |  |  |
|  |  |  |

空白行预留给后续接入渠道；新增支持后会补充接入方式和部署要求。

## 功能

- 按群过滤消息，识别群主、管理员、重点人物、@全体成员和紧急关键词。
- 默认静默 2 分钟后处理普通消息；持续活跃的群最多等待 10 分钟，紧急消息立即处理。
- 首次启动通过网页向导设置管理密码，可选配置 LLM 和首个信息源；新安装默认无信息源，管理密码始终可登录。
- 同一信息源可包含多个独立账号，各自维护连接、监听群、主人和规则，通知按来源与账号分类。
- 支持 QQ 私聊、ntfy 和 Bark 通知；可按账号及等级广播到多个目标。
- 实时通知、通知归档、关键词检索和 LLM 历史查询。
- 阅读 JSON/XML 卡片、群公告、合并转发和同群引用内容；解析有深度、请求数和超时上限。
- 完整权限主人可私聊机器人查询通知、群内待办和来源上下文；通知型账号不能使用查询助手。
- 可选接入 Jev 做消息筛选、LLM 做内容提炼、MinerU 解析文档，以及 Cloudflare R2 / S3 备份通知归档。
- 通过标准 MCP 或配套 API Skill，让第三方 Agent 使用有账号范围限制的 Bearer Key 只读检索。

## Docker 部署

当前发布镜像支持 NapCat / OneBot v11，需要一台能访问其 HTTP 和 WebSocket 接口的 Docker 主机。[GHCR 镜像](https://github.com/orgs/KoinaAI/packages/container/package/group-monitor)公开可拉取；将 `VERSION` 替换为[GitHub Release](https://github.com/KoinaAI/Group-Monitor/releases)使用的 tag。

```bash
VERSION=v1.0.0

docker run -d \
  --name group-monitor \
  --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  -v group-monitor-data:/data \
  "ghcr.io/koinaai/group-monitor:${VERSION}"
```

打开 <http://127.0.0.1:8080>。数据卷 `/data` 保存配置和正式通知归档，容器更新或重建时请保留该卷。

### 首次配置

1. 从启动日志读取一次性设置令牌（Docker 使用 `docker logs group-monitor`），或在启动前通过 `NAP_SETUP_TOKEN` 指定。打开网页，输入令牌并设置 10–72 字节的管理密码。密码只保存 bcrypt 哈希；完成后不能再次认领实例。
2. 向导中可选填写 LLM 凭据和首个 NapCat 账号，也可跳过。新安装不会自动连接任何信息源。
3. 在「信息源」新增 NapCat 来源及账号，填写从应用容器可访问的 OneBot HTTP / WebSocket 地址和令牌。顶部选择账号后，在「群聊」「主人」「规则」「连接」管理该账号。不同账号即使群号相同，也各自处理和归档。
4. 需要 QQ 通知时，在该账号的「主人」中完成私聊验证码绑定。需要外部推送时，在「广播通知」添加 ntfy 或 Bark。按需要启用 Jev、附件阅读和归档备份。

管理密码不受 NapCat 在线状态影响。已有配置升级时，原连接、监听群、主人和规则迁移到 `legacy-napcat / legacy-default`，已有归档仍可读取；初始化向导可为旧实例设置永久管理密码。`NAP_PASSWORD` 保留为旧版应急入口，仅在无可用主人、NapCat 离线或验证码不可用时生效。

规则默认继承全局设置，修改账号规则后使用独立规则。LLM、Jev、附件解析、备份及外部通知目标在实例内共享；外部目标可进一步限制适用账号。暂时只支持 `napcat` 渠道类型，来源与账号结构为后续渠道保留扩展位置。

Docker 示例只把端口绑定到宿主机回环地址。若要从公网访问，请放在 HTTPS 反向代理后，并限制可访问来源。服务重启后，所有登录会话都会失效。

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `NAP_ADDR` | `127.0.0.1:8787` | Go 后端监听地址；Docker 镜像内由 Nginx 反代，通常无需修改 |
| `NAP_CONFIG` | `config.json` | 配置文件路径；Docker 镜像使用 `/data/config.json` |
| `NAP_NOTICE_DIR` | 配置文件同目录下的 `notices/` | 通知归档目录；Docker 默认 `/data/notices` |
| `NAP_SETUP_TOKEN` | 启动时随机生成并写入日志 | 首次网页初始化令牌；已初始化实例不再接受初始化 |
| `NAP_PASSWORD` | 未设置 | 旧版应急登录密码；与向导设置的永久管理密码不同 |
| `NAP_SESSION_HOURS` | `12` | 会话有效时长，单位为小时 |

### 通知归档、文件与备份

正式通知按日保存为 JSONL。默认保留 90 天，归档总量上限为 256 MiB；不会保存完整群聊、附件二进制或临时下载链接。开启归档需要在「智能」中配置有效的 Jev API Key；没有可用 Jev 时，实时通知仍可继续，但不会写入正式通知归档。启用 Jev 或 LLM 会把相关消息文本发送到你配置的服务；启用 MinerU 时，受支持的附件会上传至 MinerU 解析。

附件阅读默认关闭。TXT、Markdown 和 DOCX 在本地解析；PDF、DOC、PPT、XLS 等格式通过 [MinerU v4 API](https://mineru.net/apiManage/docs) 处理，需要配置其 API Key。默认单文件上限为 20 MiB，每条消息最多读取 3 个附件，原始文件不会写入归档。

云端备份默认关闭，可选 Cloudflare R2 或其他 S3 兼容存储；启用后只上传完整的通知归档分片，不包含配置和密钥。默认计划为每天 03:00；容器通常使用 UTC，可在计划表达式中指定 `CRON_TZ=Asia/Shanghai`。

### 主人私聊查询

配置并启用 LLM 后，完整权限主人可向机器人私聊「最近有哪些通知」「班群本周要交什么材料」或「这条通知的原文是什么」。助手优先检索正式通知归档，必要时临时读取已监听群最近最多 120 条消息，或源消息前后各最多 5 条上下文。未监听群不参与查询，查询结果不会写入聊天数据库。

每次提问独立处理，不保留私聊对话历史；追问时请带上群名或事项。日期按北京时间解释，查无结果仅代表本次有限查询范围内未找到。每位主人同时处理一个问题，总计最多四个；处理中的重复提问和超长消息会被忽略。模型需支持 OpenAI 兼容的 function tools。

### ntfy / Bark 广播

在「广播通知」添加目标，保存后可发送测试通知：

- **ntfy**：填写服务器地址（如 `https://ntfy.sh`）、主题，以及私有主题需要的访问令牌。
- **Bark**：填写服务地址和设备 Key，可指定消息分组；支持自托管服务。

同一通知会发往所有符合最低等级和账号范围的启用目标；不绑定 QQ 主人也可接收外部推送。账号范围留空表示全部当前及未来账号。一个目标失败不阻止其他目标发送。令牌及设备 Key 保存后不再回显，留空保存保留原值。

### MCP 与 Bearer API

在「Agent 接入」生成 API Key，选择允许读取的账号。密钥只展示一次，服务端只存 SHA-256 摘要；留空账号范围表示全部当前及未来账号，可随时撤销。此权限仅能检索当前监听群，不能发送消息、修改配置或获取连接凭据。

- **MCP 地址**：`https://你的实例/api/mcp`
- **传输**：无状态 Streamable HTTP，POST 返回 JSON；不提供 SSE server push 或 Session ID。
- **鉴权**：每个请求带 `Authorization: Bearer <API_KEY>`，网页 Cookie 不能代替 API Key。
- **协议版本**：`2025-03-26`、`2025-06-18`、`2025-11-25`。

服务在 `initialize.instructions` 说明检索用途，每个工具包含描述、参数 Schema 和只读标记。使用官方 TypeScript MCP SDK 时，可从环境变量配置：

```js
import { Client } from '@modelcontextprotocol/sdk/client/index.js'
import { StreamableHTTPClientTransport } from '@modelcontextprotocol/sdk/client/streamableHttp.js'

const client = new Client({ name: 'notice-reader', version: '1.0.0' })
await client.connect(new StreamableHTTPClientTransport(
  new URL('/api/mcp', process.env.XUNSHU_URL),
  { requestInit: { headers: { Authorization: `Bearer ${process.env.XUNSHU_API_KEY}` } } },
))
const sources = await client.callTool({ name: 'list_sources', arguments: {} })
```

| 工具 | 用途 |
|---|---|
| `list_sources` | 获取当前密钥可读取的信息源与账号 ID |
| `list_watched_groups` | 获取指定账号当前监听群的 `group_id` |
| `recent_notices` | 查询最近正式通知，可按群过滤和分页 |
| `search_notices` | 按关键词或连续短语搜索正式通知 |
| `search_group_history` | 查询监听群最近最多 120 条消息中的匹配内容 |
| `search_message_by_uuid` | 读取源消息及前后各最多 5 条上下文 |

除 `list_sources` 外都需要 `account_id`。先调用它选择账号，再调用 `list_watched_groups` 选择群。正式归档不表示成功送达，历史检索是有限窗口，查无结果不表示全历史不存在。

不使用 MCP 时，可访问相同权限的 JSON API：`GET /api/agent/v1/tools` 发现工具，`POST /api/agent/v1/query` 执行查询。请求头同样使用 Bearer Key，POST 示例：

```json
{"name":"recent_notices","arguments":{"account_id":"school","limit":5}}
```

`school` 替换为真实账号 ID。工具错误返回 `400`；无效或撤销的 Key 返回 `401`；实例最多允许 8 个并发 Agent 请求，超额返回 `429`。单次请求上限 30 秒。公网部署使用 HTTPS；现有 Docker / Vite 的 `/api` 代理已覆盖 MCP 和 API。

### 配套 API Skill

打开控制台「Agent 接入」→「安装 Skill」，选择 Codex 或 Claude Code，复制安装命令并在运行 Agent 的电脑上执行（需要 Python 3 和 curl）。安装完成后重新打开 Agent。也可直接下载完整 ZIP，解压后将 `xunshu-api` 文件夹放入其他 Agent 的 Skill 目录。

Skill 随前端静态资源部署，源文件位于 [frontend/public/skills/xunshu-api](frontend/public/skills/xunshu-api/SKILL.md)。每次开发启动和构建都会生成 `/skills/xunshu-api.zip` 与 `/skills/install.py`，Docker 镜像中同样可用，不依赖 GitHub 下载。安装器拒绝覆盖已有目录；更新时先将原 Skill 文件夹改名备份，再执行安装命令。

Codex 默认安装到 `~/.codex/skills/xunshu-api`（支持 `CODEX_HOME`），Claude Code 安装到 `~/.claude/skills/xunshu-api`。安装文件不含实例密钥；在 Agent 的执行环境设置 `XUNSHU_URL` 和 `XUNSHU_API_KEY`，然后使用 Skill 中的 Python 标准库客户端，无需 MCP SDK。不要把真实密钥写进仓库或 Skill。

```bash
# 在已安装的 xunshu-api 目录下执行
python3 scripts/query.py tools
python3 scripts/query.py query list_sources
python3 scripts/query.py query recent_notices '{"account_id":"school","limit":5}'
```

Skill 包含工具选择、查询范围、游标分页、来源引用及不可信消息处理说明。客户端拒绝重定向，限制响应大小，并且不会打印密钥。

## 从源码运行

需要 Go 1.25+、Node.js 22+，以及当前支持的 NapCat / OneBot v11 接口。后端与前端分别在两个终端启动：

```bash
# 终端一：在仓库根目录启动后端，默认监听 127.0.0.1:8787
./run.sh
```

```bash
# 终端二：安装并授权 HeroUI Pro，再启动前端
cd frontend
npm ci
HEROUI_KEY='your-HeroUI-key' npx -y hpsetup@latest react --auto
npm run dev
```

前端开发服务器默认在 <http://localhost:5173>，并将 `/api` 代理到本机 Go 后端。`HEROUI_KEY` 只用于获取授权的 Pro 组件，不要提交到 Git。

## 检查与发布

后端检查：

```bash
cd backend
test -z "$(gofmt -l .)"
go vet ./...
go test -race -coverprofile=coverage.out ./...
```

前端检查：

```bash
cd frontend
npm run lint
npm run build
```

GitHub Actions 会在 push 和 pull request 时分别运行 [Backend CI](.github/workflows/backend-ci.yml) 与 [Frontend CI](.github/workflows/frontend-ci.yml)。前端 CI 使用仓库 Actions secret `HEROUI_KEY` 获取 Pro 组件。

发布 GitHub Release 后，Docker 发布 workflow 会构建 `linux/amd64` 镜像并推送到 GHCR，镜像 tag 与 Release tag 相同。也可以在 Actions 页面手动运行 [Docker 发布 workflow](.github/workflows/docker-publish.yml)。普通 push 不会发布 Docker 镜像。
