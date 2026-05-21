# NJK Go 仓库导航

## 目的

本文件面向后续 agent 和开发者，提供当前 Go 仓库的长期导航信息：

- 说明项目的真实结构和主运行链路
- 标出常见改动应该落在哪些文件
- 记录容易踩坑的实现细节
- 明确哪些模块是活跃代码，哪些更像保留产物

本文件描述的是“当前代码现状”，不是某一次需求的临时计划。

## 项目概览

这是一个基于 NapCat 反向 WebSocket 的群聊机器人服务，当前主能力包括：

- 接收群消息、notice、action 回执
- 解析消息段并落库到 PostgreSQL
- 执行正则命令体系
- 调用 AI 接口生成回复
- 调用 BBH HTTP 服务处理 `.bbh` 相关命令
- 对图片做 pHash 消重
- 从历史消息里提取系统表情并发送 `set_msg_emoji_like`

程序入口是 `cmd/server/main.go`。

## 目录地图

### 运行主链

- `cmd/server/main.go`
  - 读取配置
  - 连接 PostgreSQL
  - 构造 `bot.Service`
  - 启动 WebSocket 服务

### 核心业务

- `internal/bot/`
  - 机器人核心逻辑
  - 命令定义、命令分发、消息入库、消息发送、图片处理、报告生成都在这里

### 协议层

- `internal/napcat/`
  - NapCat 入站事件解析
  - NapCat 请求/响应/消息段类型定义

### 传输层

- `internal/transport/ws/`
  - 自研 WebSocket server
  - 完成握手、读写 frame、分发入站消息

### 外部服务

- `internal/ai/`
  - OpenAI-compatible chat completions client
- `internal/bbh/`
  - BBH HTTP client

### 配置与数据

- `internal/config/`
  - `.env` 与环境变量读取
- `sql/create_njk_tables.sql`
  - 当前数据库 schema
- `internal/model/`
  - gorm/gen 生成的 model
- `internal/query/`
  - gorm/gen 生成的 query 层，目前主业务基本未直接使用

### 其他

- `internal/client/db.go`
  - 当前主流程未引用，可视为保留文件

## 启动链路

启动顺序如下：

1. `config.Load()` 读取配置
2. `gorm.Open(postgres.Open(cfg.DSN()))` 建立数据库连接
3. `bot.NewService(...)` 装配业务依赖
4. `ws.NewServer(cfg.ListenAddr, service)` 创建 WS 服务
5. `ListenAndServe()` 开始接收 NapCat 推送

关键文件：

- `cmd/server/main.go`
- `internal/config/config.go`
- `internal/bot/service.go`
- `internal/transport/ws/server.go`

## 入站消息主链路

NapCat 入站 JSON 的处理流程：

1. `internal/transport/ws/server.go`
   - 接收文本帧
   - 调用 `napcat.ParseInboundMessage`
2. `internal/napcat/parser.go`
   - 识别 `group_message` / `notice` / `action_response`
3. `internal/bot/service_ingress.go`
   - `HandleGroupMessage`
   - `HandleNotice`
   - `HandleActionResponse`

### Group Message

群消息入口在 `HandleGroupMessage`，主要步骤：

1. 群白名单过滤
2. 用户黑名单过滤
3. 对 `event.RawMessage` 做命令匹配
4. 如果没有命令但 `@bot`，回退为 `commandNJK`
5. 按条件决定是否先入库
6. 执行命令或随机回复
7. 统一发送文本消息或表情点赞动作

重要事实：

- 不是所有入站群消息都会先落库
- 目前只有“未命中命令”或“命中 `commandNJK`”时，才会先走 `saveIncomingMessageAndCheckImages`
- 这会影响“命令是否能在数据库里看到当前触发消息”的判断

## 消息入库链路

入站消息保存函数是 `internal/bot/service_ingest.go` 里的 `saveIncomingMessageAndCheckImages`。

它会做这些事：

1. `UpsertUser`
2. `UpsertGroup`
3. 遍历 `event.Message.Segments`
4. 提取：
   - `reply`
   - `at`
   - `text`
   - `image`
5. 将完整 segments `json.Marshal` 后写入 `message.raw_json`
6. 将 `event.RawMessage` 写入 `message.raw_message`
7. 保存 `at_user`
8. 对图片做消重与白名单处理

### raw_json 语义

- 入站群消息的 `raw_json` 通常是 `[]napcat.MessageSegment` 的 JSON 数组
- 机器人自己发出去并被回执补写入库的消息，`raw_json` 可能只是一个被 JSON 编码的纯文本字符串

因此：

- 任何读取历史 `raw_json` 的逻辑，都不能假设它永远能反序列化成 `[]napcat.MessageSegment`
- 遇到字符串型或非法 JSON 时，应安全跳过，而不是让整条链路失败

## 出站消息主链路

### 文本消息

文本发送走 `sendGroupText`：

1. 规范化文本
2. 组装 NapCat `send_group_msg`
3. 发到 WebSocket
4. 推入 `pendingQueue`

只有文本消息会进入 `pendingQueue`。

### action 动作

例如 `.face` 用到的 `set_msg_emoji_like`，走独立动作发送，不进入 `pendingQueue`。

这意味着：

- 不等待文本式的消息回执
- 不触发“保存机器人自己发言”逻辑
- 不会新增 `message` 表记录

### 回执处理

`HandleActionResponse` 当前主要服务于 `send_group_msg`：

- 只处理成功回执
- 期待 `action.Data` 里能解析出 `message_id`
- 然后从 `pendingQueue` 弹出一条待确认文本
- 如果 `ShouldSave=true`，调用 `saveSelfMessage`

所以：

- 不要把没有 `message_id` 的 action 误纳入这条链路
- 给 NapCat 新增动作时，先判断它是否应该进入 `pendingQueue`

## 命令系统

### 命令定义位置

- `internal/bot/prompts.go`
  - `commandKey`
  - `commandDefs`
  - `helpText`
  - AI 命令的 `SystemPrompt`

### 命令编译位置

- `internal/bot/service.go`
  - `NewService`
  - 把 `commandDefs` 编译成正则
  - 建立 `commands` 和 `commandMap`

### 命令匹配与分发

- `internal/bot/commands.go`
  - `matchCommand`
  - `commandByKey`
  - `buildCommandHandler`

### 当前命令大类

- AI 总结类
  - `.概括`
  - `.总结`
  - `.俳句`
  - `.无只因`
  - `.最`
  - `.vs`
  - `.ccb`
  - `.xmas`
- 对话类
  - `.ai`
  - `.aic`
  - 自动 `NJK` 回复
- 历史/统计类
  - `.报告`
  - `.face`
- 工具类
  - `.XdY` 掷骰子，例如 `.2d6`、`.2 d 6`
- 帮助类
  - `.help`
  - `.help bbh`
- BBH 类
  - `.bbh`
  - `.bbh <bookID>`
  - `.bbh <bookID> <para>`
  - `.bbh <bookID> <left>-<right>`
  - `.bbh <bookID> add ...`
  - `.bbh <bookID> ai`

### 新增命令的常见改法

如果要新增一个普通命令，通常需要改这些地方：

1. `prompts.go`
   - 新增 `commandKey`
   - 在 `commandDefs` 里加正则
   - 如需展示，更新 `helpText`
2. `commands.go`
   - 在 `buildCommandHandler` 增加分支
3. `internal/bot/command_xxx.go`
   - 新增或扩展 handler
4. `service_test.go`
   - 补命令匹配或 handler 行为测试

### 输出是否保存

命令 handler 里常用两种返回方式：

- `simpleOutbound(...)`
  - 仅发出，不保存机器人回复
- `savedReplyOutbound(...)`
  - 发出后等待回执，并把机器人回复补写入库

如果需求明确要求“输入输出都不保存”，通常意味着：

- 该命令不应走 `saveIncomingMessageAndCheckImages`
- handler 应返回 `simpleOutbound(...)`

## 历史消息读取

当前主历史读取接口在 `internal/bot/store.go`：

- `RecentMessages`
- `MessagesSince`

`StoredMessage` 当前已经包含：

- `message_id`
- `time`
- `sender_id`
- `nickname`
- `card`
- `text`
- `raw_message`
- `raw_json`

因此：

- 许多新功能不需要再专门扩展 store 才能拿到 `raw_json`
- 实现前先确认现有 `StoredMessage` 是否已经足够

## 图片子系统

图片逻辑集中在 `internal/bot/image.go`。

当前行为：

- 下载图片
- 计算 pHash
- 保存到 `image` 表
- 在同群历史里做相似图查重
- 动画表情图走白名单逻辑

实现注意：

- 图片查重函数是“保存并查重”，不是单纯“只查不存”
- 如果要改变查重时机，必须同时考虑：
  - `image.message_id -> message.message_id` 的外键约束
  - 当前消息内多张图片互相影响的问题
  - `GroupImageCandidates` 的过滤条件

## .face 功能现状

`.face` 已经落地，不再是待实现需求。

当前实现要点：

- 命令在 `prompts.go` 中注册
- handler 在 `internal/bot/command_face.go`
- 从本地数据库最近消息里读取 `raw_json`
- 解析 `[]napcat.MessageSegment`
- 提取 `segment.Type == "face"` 的 `data.id`
- 通过 `set_msg_emoji_like` 逐个发送
- 不入 `pendingQueue`
- 不落库机器人动作

额外注意：

- 当前正则允许 `.face12`
- 如果后续要调整语义，不要只改文档，必须同步更新正则和测试

## 骰子命令现状

当前已支持 `.XdY`，例如：

- `.2d6`
- `.2 d 6`

实现位置：

- 正则定义在 `prompts.go`
- handler 在 `internal/bot/command_dice.go`

当前语义：

- `x` 为掷骰子次数
- `y` 为骰子面数
- `x` 限制 `<= 20`
- 输出为若干随机整数，用 `", "` 连接
- 使用 `simpleOutbound(...)`

## 报告与旧表

数据库 schema 里仍存在：

- `topic`
- `word`
- `msg_topic`
- `msg_word`

但当前报告输出已经不展示“热门话题/高频词汇”部分。

因此：

- 这些表目前更像保留 schema
- 改报告功能时，不要先入为主地认为这些表在活跃使用

## 外部集成

### AI

- 客户端在 `internal/ai/client.go`
- 使用 OpenAI-compatible `chat/completions`
- `Service` 同时注入了 `aiClient` 和 `freeAIClient`
- 目前 `freeAIClient` 基本未成为核心路径，改动前先确认是否真的被使用

### BBH

- 客户端在 `internal/bbh/client.go`
- 命令处理在 `internal/bot/command_bbh.go`
- 当前配置结构里已经支持 `BBHBaseURL`

注意：

- `.env.example` 当前没有列出 `BBH_BASE_URL`
- 如果要完善新手接入体验，记得同时更新配置示例和文档

## 配置说明

配置加载在 `internal/config/config.go`。

当前关键配置包括：

- `WS_ADDR`
- `DB_HOST`
- `DB_PORT`
- `DB_USER`
- `DB_PWD`
- `DB_NAME`
- `API_KEY`
- `BASE_URL`
- `MODEL_NAME`
- `FREE_MODEL_NAME`
- `BBH_BASE_URL`
- `BOT_USER_ID`
- `BOT_NICKNAME`
- `GROUP_IDS`
- `BANNED_USER_IDS`

额外注意：

- `GROUP_IDS` 是群白名单
- `BANNED_USER_IDS` 是用户黑名单
- `BBH_BASE_URL` 在代码里已支持，但示例配置文件可能未同步

## 测试布局

当前测试主要分布如下：

- `internal/bot/service_test.go`
  - 命令匹配
  - 报告格式
  - `.face`
  - 骰子命令
  - 黑名单过滤
- `internal/bot/image_test.go`
  - hash 兼容相关
- `internal/config/config_test.go`
  - 配置加载
- `internal/napcat/parser_test.go`
  - NapCat 解析
- `internal/transport/ws/server_test.go`
  - WebSocket 处理与并发写

目前测试缺口：

- `Store` 与 SQL 层缺少直接测试
- AI / BBH HTTP client 缺少测试
- 图片消重缺少高层集成测试

另外：

- `config_test.go` 里可能存在过时断言
- 改配置逻辑时，先核对实现和测试谁才是最新事实

## 常见改动导航

### 想加一个新命令

优先看：

- `internal/bot/prompts.go`
- `internal/bot/commands.go`
- `internal/bot/command_*.go`
- `internal/bot/service_test.go`

### 想改消息保存逻辑

优先看：

- `internal/bot/service_ingest.go`
- `internal/bot/store.go`
- `sql/create_njk_tables.sql`

### 想改消息发送逻辑

优先看：

- `internal/bot/service_ingress.go`
- `internal/bot/state.go`
- `internal/napcat/types.go`

### 想改协议解析

优先看：

- `internal/napcat/parser.go`
- `internal/napcat/types.go`
- `internal/transport/ws/server.go`

### 想改 AI 相关行为

优先看：

- `internal/bot/command_ai.go`
- `internal/ai/client.go`
- `internal/bot/prompts.go`

### 想改图片逻辑

优先看：

- `internal/bot/image.go`
- `internal/bot/service_ingest.go`
- `internal/bot/store.go`

## 已知注意事项

### 1. 命令消息不一定入库

不要默认当前触发命令一定能在数据库最近消息里查到。

### 2. raw_json 不是同一种形态

历史消息里既可能是 segments 数组，也可能是机器人自发文本的 JSON 字符串。

### 3. action 回执并不统一

`HandleActionResponse` 目前更偏向处理 `send_group_msg` 的成功回执，不要拿它生搬硬套所有 NapCat action。

### 4. 当前存在一些保留模块

- `internal/query/` 多为生成代码，当前主流程未直接依赖
- `internal/client/db.go` 当前未接入
- topic/word 相关表目前不是主业务热点

### 5. 共享随机源是并发共享的

`Service` 里有共享的 `rng`。如果后续新增强依赖随机数且并发敏感的逻辑，记得先评估并发安全与可测试性。

## 推荐阅读顺序

第一次接手本仓库时，建议按这个顺序阅读：

1. `cmd/server/main.go`
2. `internal/config/config.go`
3. `internal/bot/service.go`
4. `internal/bot/service_ingress.go`
5. `internal/bot/service_ingest.go`
6. `internal/bot/commands.go`
7. `internal/bot/prompts.go`
8. `internal/bot/store.go`
9. `internal/napcat/types.go`
10. `internal/napcat/parser.go`
11. `internal/transport/ws/server.go`

如果是按功能阅读：

- 命令功能：`prompts.go` -> `commands.go` -> `command_xxx.go`
- 入库功能：`service_ingest.go` -> `store.go` -> `sql/create_njk_tables.sql`
- 出站功能：`service_ingress.go` -> `state.go` -> `napcat/types.go`
- 图片功能：`service_ingest.go` -> `image.go` -> `store.go`

## 文档维护原则

后续修改本文件时，优先保持以下原则：

- 描述当前实现，不写一次性任务计划
- 重点写“入口、职责、调用关系、坑点”
- 对已实现功能，避免继续用“待实现”“缺口”表述
- 对保留代码和历史残留，要明确标注“当前未活跃使用”
