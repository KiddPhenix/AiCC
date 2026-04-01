# 个人 AI 解决方案库

一个尽量简单、可运行的 MVP：

- 后端：Go
- 存储：SQLite
- 接口：REST API
- 前端：最简单的静态 Web 页面
- 桌面应用：Windows 单 exe，包含 Web 服务、抓取器和系统托盘
- AI 识别：将抓取内容提取成结构化总结、分类、标签、时间和来源

## 1. 项目目录结构建议

```text
AiHelper/
├─ cmd/
│  └─ server/                 # 程序入口
├─ internal/
│  ├─ domain/                 # 核心模型与校验
│  ├─ httpapi/                # HTTP Handler / 路由
│  ├─ repository/             # 数据访问层
│  ├─ service/                # 业务逻辑
│  ├─ source/                 # 来源适配器与导入模型
│  └─ storage/                # SQLite 初始化与迁移
├─ web/
│  └─ static/                 # 最简单的前端页面
├─ data/                      # SQLite 文件
├─ go.mod
└─ README.md
```

说明：

- `domain` 保持业务概念稳定，未来换数据库或接入更多来源时不容易乱。
- `repository` 先只做 SQLite，后面可以继续加别的实现。
- `source` 预留给对话平台、文档系统、工单系统等来源适配器。
- `web/static` 先保留最轻量方案，后面再替换成正式前端也不影响后端结构。

## 2. 数据库表设计

### `solutions`

| 字段 | 类型 | 说明 |
|---|---|---|
| `id` | INTEGER PK | 主键 |
| `title` | TEXT | 方案标题 |
| `problem` | TEXT | 问题背景 |
| `solution_text` | TEXT | 解决方案正文 |
| `efficacy_source` | TEXT | 有效性来源：`manual` / `auto` |
| `status` | TEXT | 状态：`draft` / `probable_success` / `confirmed_success` / `failed` |
| `source_type` | TEXT | 来源类型，如 `chat` / `doc` |
| `source_name` | TEXT | 来源名称 |
| `source_ref` | TEXT | 来源引用，如 URL / ID |
| `raw_content_ref` | TEXT | 原始内容引用 |
| `dedup_key` | TEXT UNIQUE | 去重指纹 |
| `created_at` | DATETIME | 创建时间 |
| `updated_at` | DATETIME | 更新时间 |

### `solution_tags`

| 字段 | 类型 | 说明 |
|---|---|---|
| `solution_id` | INTEGER | 关联方案 |
| `tag` | TEXT | 标签 |

说明：

- 标签拆表，便于后续过滤和统计。
- `dedup_key` 用标题、问题、方案、来源、标签做归一化后哈希，先满足 MVP 去重。
- 关键词搜索当前走 `LIKE`，先保证简单可用，后续需要更强搜索时再升级到 FTS。

## 3. REST API 设计

### `POST /api/v1/solutions`

新增方案记录。

请求示例：

```json
{
  "title": "用结构化提示词稳定生成 PRD 草稿",
  "problem": "AI 输出格式不稳定，复用困难",
  "solution_text": "固定输入模板 + 输出字段约束 + 让 AI 先澄清后生成",
  "efficacy_source": "manual",
  "status": "confirmed_success",
  "source_type": "chat",
  "source_name": "ChatGPT 会话",
  "source_ref": "session-123",
  "raw_content_ref": "note://prd/001",
  "tags": ["prompt", "prd", "writing"]
}
```

返回：

- `201 Created`：创建成功
- `409 Conflict`：检测到重复，返回已存在记录
- `400 Bad Request`：参数不合法

### `GET /api/v1/solutions`

查询方案列表。

支持参数：

- `q`：关键词搜索
- `status`：按状态过滤
- `efficacy_source`：按有效性来源过滤
- `tag`：按标签过滤
- `limit` / `offset`：分页

请求示例：

```text
GET /api/v1/solutions?q=prompt&status=confirmed_success
```

说明：

- 先做列表查询，满足 MVP 检索。
- 明细接口、更新接口、删除接口可以等下一期再补。

### `POST /api/v1/imports`

自动导入方案候选。

当前已实现第一个 adapter：`local_json`

请求示例：

```json
{
  "adapter": "local_json",
  "path": "data/examples/local_import.sample.json",
  "default_status": "probable_success",
  "default_tags": ["imported", "auto"]
}
```

返回示例：

```json
{
  "adapter": "local_json",
  "path": "data/examples/local_import.sample.json",
  "imported": 2,
  "duplicates": 0,
  "skipped": 0
}
```

说明：

- `local_json` 支持两种格式：JSON 数组、JSONL。
- 导入记录统一写入 `efficacy_source=auto`。
- 候选项里没有 `status` 时，会使用 `default_status`，默认是 `probable_success`。
- 重复记录不会报错中断，而是记入 `duplicates`。

## 4. 已实现代码

当前已经完成：

- Go 服务启动与路由
- SQLite 自动建表
- 新增方案记录
- 自动来源导入的第一个 adapter：`local_json`
- 抓取记录的 AI 结构化分析
- 有效性来源与状态校验
- 标签存储
- 关键词搜索
- 去重检测
- 最简单 Web 页面录入与查询

## 5. 每一步简短说明

1. 目录结构：先把“业务、存储、接口、前端”分开，避免后面加功能时互相缠住。
2. 表设计：只保留 MVP 必需字段，先可用，再演进。
3. API 设计：先做“新增 + 查询”两条主链，覆盖核心使用场景。
4. 代码实现：优先把端到端链路跑通，不提前做复杂抽象。
5. 前端页面：用静态页面直接验证产品流程，减少前期成本。
6. 去重与搜索：先用简单稳定方案，后续有规模再升级。
7. 自动导入：先做本地文件 adapter，把“多来源接入”的骨架先搭起来。

## 6. 运行方式

```bash
go run cmd/server/main.go
```

默认地址：

- Web 页面：[http://localhost:8090](http://localhost:8090)
- SQLite 文件：`data/aihelper.db`
- 导入示例文件：`data/examples/local_import.sample.json`

可选环境变量：

- `AIHELPER_ADDR`：服务监听地址
- `AIHELPER_DB_PATH`：数据库文件路径

## 7. Windows 桌面应用

现在推荐直接使用单个桌面应用，不再分别启动 Web 和抓取程序。

特点：

- 单个 exe 同时运行 Web 服务、抓取器和托盘
- 运行时不显示 cmd 窗口
- 连续两次 `Ctrl+C` 后抓取当前剪贴板文本
- 系统托盘右键支持：
  `打开知识库`
  `重启`
  `关闭`

## 8. 一键启动脚本

推荐直接使用脚本统一管理 Web 服务和抓取工具：

```powershell
pwsh -File scripts/aihelper.ps1 start
```

常用命令：

```powershell
pwsh -File scripts/aihelper.ps1 start
pwsh -File scripts/aihelper.ps1 stop
pwsh -File scripts/aihelper.ps1 restart
pwsh -File scripts/aihelper.ps1 status
pwsh -File scripts/aihelper.ps1 logs
pwsh -File scripts/aihelper.ps1 debug
```

说明：

- `start` 会同时启动 Web 服务和抓取工具
- `start` 会启动单个桌面应用 exe
- Web 默认运行在 `8090`
- `stop` 会关闭这个桌面应用
- `logs` 会实时输出运行日志，按 `Ctrl+C` 退出查看
- `debug` 会先启动服务，再立即进入实时日志模式
- 运行日志保存在：
  `data/runtime/app.out.log`
  `data/runtime/app.err.log`

## 9. 调试模式

如果你想直接看运行时日志，推荐：

```powershell
pwsh -File scripts/aihelper.ps1 debug
```

如果服务已经在跑，只想看日志：

```powershell
pwsh -File scripts/aihelper.ps1 logs
```

当前日志会包含：

- Web 请求日志：方法、路径、状态码、耗时
- 抓取日志：是否成功抓取、内容类型、字符数、是否重复
- 错误日志：服务启动失败、端口占用、AI 分析失败等

## 10. AI 分析抓取内容

推荐把 AI 配置写在 `config/app.json`：

```json
{
  "ai": {
    "base_url": "https://api.openai.com/v1",
    "api_key": "你的 OpenAI API Key",
    "model": "gpt-4o-mini"
  }
}
```

仓库里已经提供模板：

- [app.example.json](D:/Work/tools/AiHelper/config/app.example.json)
- [app.json](D:/Work/tools/AiHelper/config/app.json)

如果你不想改文件，也仍然可以用环境变量覆盖：

```powershell
$env:OPENAI_API_KEY="你的 OpenAI API Key"
```

可选模型设置：

```powershell
$env:OPENAI_MODEL="gpt-4o-mini"
```

然后启动：

```powershell
pwsh -File scripts/aihelper.ps1 start
```

使用方式：

1. 用抓取工具保存一段内容
2. 打开 [http://127.0.0.1:8090](http://127.0.0.1:8090)
3. 在“抓取记录”里点击 `AI 分析`
4. 页面会生成结构化结果，包括标题、总结、分类、问题、建议沉淀、时间、来源、标签、建议状态
5. 如果结果合适，再点击 `转成方案`

当前 AI 分析会把结果缓存到本地数据库，避免重复分析同一条抓取记录。
