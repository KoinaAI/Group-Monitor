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
- 当前通过 QQ 私聊验证码登录；NapCat 离线或没有可用主人时，可选用应急密码登录。
- 实时通知、通知归档、关键词检索和 LLM 历史查询。
- 阅读 JSON/XML 卡片、群公告、合并转发和同群引用内容；解析有深度、请求数和超时上限。
- 可选接入 Jev 做消息筛选、LLM 做内容提炼、MinerU 解析文档，以及 Cloudflare R2 / S3 备份通知归档。

## Docker 部署

当前发布镜像支持 NapCat / OneBot v11，需要一台能访问其 HTTP 和 WebSocket 接口的 Docker 主机。[GHCR 镜像](https://github.com/orgs/KoinaAI/packages/container/package/group-monitor)公开可拉取；将 `VERSION` 替换为[GitHub Release](https://github.com/KoinaAI/Group-Monitor/releases)使用的 tag。

```bash
VERSION=v1.0.0

docker run -d \
  --name group-monitor \
  --restart unless-stopped \
  -p 127.0.0.1:8080:8080 \
  -v group-monitor-data:/data \
  -e NAP_PASSWORD='replace-with-a-strong-password' \
  "ghcr.io/koinaai/group-monitor:${VERSION}"
```

打开 <http://127.0.0.1:8080>。数据卷 `/data` 保存配置和正式通知归档，容器更新或重建时请保留该卷。

### 首次配置

1. 首次启动建议设置 `NAP_PASSWORD`。尚未配置主人或 NapCat 不可用时，可用它登录控制台。
2. 当前版本在「连接」中填写从应用容器可访问的 NapCat OneBot HTTP 地址、WebSocket 地址和访问令牌，并确认连接成功。
3. 在「主人」中绑定至少一个完整权限的主人账号。验证码会由 NapCat 私聊发送给待绑定账号；通知型账号不能用于控制台验证码登录。
4. 在「群聊」中选择要监听的群，再按需要配置规则、Jev 和 LLM。

当验证码登录可用时，应急密码登录会关闭；应急密码仅在没有主人、NapCat 离线或验证码无法送达时启用。请妥善保管密码和 OneBot 令牌。

Docker 示例只把端口绑定到宿主机回环地址。若要从公网访问，请放在 HTTPS 反向代理后，并限制可访问来源。服务重启后，所有登录会话都会失效。

### 环境变量

| 变量 | 默认值 | 说明 |
|---|---|---|
| `NAP_ADDR` | `127.0.0.1:8787` | Go 后端监听地址；Docker 镜像内由 Nginx 反代，通常无需修改 |
| `NAP_CONFIG` | `config.json` | 配置文件路径；Docker 镜像使用 `/data/config.json` |
| `NAP_NOTICE_DIR` | 配置文件同目录下的 `notices/` | 通知归档目录；Docker 默认 `/data/notices` |
| `NAP_PASSWORD` | 未设置 | 应急登录密码；OTP 可用时不接受密码登录 |
| `NAP_SESSION_HOURS` | `12` | 会话有效时长，单位为小时 |

### 通知归档、文件与备份

正式通知按日保存为 JSONL。默认保留 90 天，归档总量上限为 256 MiB；不会保存完整群聊、附件二进制或临时下载链接。开启归档需要在「智能」中配置有效的 Jev API Key；没有可用 Jev 时，实时通知仍可继续，但不会写入正式通知归档。启用 Jev 或 LLM 会把相关消息文本发送到你配置的服务；启用 MinerU 时，受支持的附件会上传至 MinerU 解析。

附件阅读默认关闭。TXT、Markdown 和 DOCX 在本地解析；PDF、DOC、PPT、XLS 等格式通过 [MinerU v4 API](https://mineru.net/apiManage/docs) 处理，需要配置其 API Key。默认单文件上限为 20 MiB，每条消息最多读取 3 个附件，原始文件不会写入归档。

云端备份默认关闭，可选 Cloudflare R2 或其他 S3 兼容存储；启用后只上传完整的通知归档分片，不包含配置和密钥。默认计划为每天 03:00；容器通常使用 UTC，可在计划表达式中指定 `CRON_TZ=Asia/Shanghai`。

## 从源码运行

需要 Go 1.22+、Node.js 22+，以及当前支持的 NapCat / OneBot v11 接口。后端与前端分别在两个终端启动：

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
