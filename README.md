# AiHelper

AiHelper 是一个给个人使用的 AI 知识沉淀工具。

它的核心目标是把你和 AI 对话里真正有价值、以后还会复用的内容沉淀下来，而不是让这些解决方案散落在聊天记录里。

当前项目已经支持：

- 双击 `Ctrl+C` 抓取当前剪贴板内容
- 用 AI 自动分析片段，提取标题、问题、方案、分类、标签、时间、来源
- 把分析结果沉淀进本地知识库
- 关键词搜索、编辑、删除、标签维护
- Windows 桌面单文件运行
- SQLite 本地存储

## 怎么用

### 1. 直接使用安装包

如果你只是想使用工具，不需要安装 Go。

可用产物：

- `dist/AiHelper-Setup.exe`
- `dist/AiHelper-installer.zip`
- `dist/AiHelper-portable.zip`

推荐：

1. 双击 `AiHelper-Setup.exe`
2. 启动后打开知识库页面
3. 如果还没配置 API Key，先进入“配置”页填写：
   - API URL
   - API Key
   - 模型
4. 之后在任意应用中连续两次按 `Ctrl+C`
5. 打开知识库查看新抓取和已沉淀内容

### 2. 从源码运行

如果你要开发或调试：

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

默认地址：

- Web: [http://127.0.0.1:8090](http://127.0.0.1:8090)

## 配置

程序支持在页面中直接配置：

- API URL
- API Key
- 模型

也可以使用配置文件模板：

- `config/app.example.json`

默认配置文件路径：

- `config/app.json`

## 适合做什么

这个工具比较适合沉淀这些内容：

- 可复用的脚本和命令
- 路径、网址、接口调用方式
- 排查问题时有效的解决步骤
- AI 生成后经验证可用的方案
- 聊天里出现的结构化工作结论

## 补充文档

更完整的设计说明、目录结构、数据库设计、API 设计和阶段性实现说明，已经迁移到：

- [whatwedo.md](whatwedo.md)
