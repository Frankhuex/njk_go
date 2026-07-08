# NJK Go 仓库导航

## 目的

本文件面向后续 agent 和开发者，描述仓库当前真实结构、主运行链路、核心职责边界与常见改动落点。

本文件描述的是“当前代码现状”，不是某一次任务计划。

## 项目概览

这是一个基于 NapCat 反向 WebSocket 的群聊机器人服务，当前主能力包括：

- 接收 `group_message`、`notice`、`action_response`
- 解析 NapCat 消息段并落库到 PostgreSQL
- 执行正则命令体系
- 调用 AI 接口生成回复
- 主模型支持多模态输入，并带有逐级回退到单模态的兜底
- 调用 BBH HTTP 服务处理 `.bbh` 相关命令
- 对图片做 pHash 消重
- 记录系统表情 ID 与消息表情回应
- 提供 `.face`、`.faceid`、`.getfaceid`、`.allface` 等系统表情相关命令
- 生成并回发对称图片
- 提供 `.生图` 命令调用 SiliconFlow 生图接口

在线服务入口是 `cmd/server/main.go`。

## 目录地图

### 运行主链

- `cmd/server/main.go`
  - 读取配置
  - 初始化 `pgstore.Store`
  - 构造 `service.Service`
  - 启动 WebSocket 服务
- `cmd/memory-factory/main.go`
  - 读取配置
  - 初始化 `pgstore.Store`
  - 构造 `service.Service`
  - 执行离线记忆生产任务
- `cmd/gen-gorm/main.go`
  - 读取配置
  - 连接数据库
  - 生成 `internal/dal/model` 与 `internal/dal/query`

### 核心业务

- `internal/service/`
  - 业务编排核心
  - 命令定义、命令分发、消息入库、图片处理、报告格式化、表情逻辑都在这里

### Handler 层

- `internal/handler/napcat/`
  - NapCat 事件入口
  - 把 NapCat 入站事件分派到 `service`
  - 把 `service` 产出的动作执行成 NapCat 请求

### 协议层

- `internal/napcat/`
  - NapCat 入站事件解析
  - NapCat 请求/响应/消息段类型定义

### 传输层

- `internal/transport/ws/`
  - 自研 WebSocket server
  - 完成握手、读写 frame、分发入站消息
  - 同时暴露 `/images/` 静态资源路由

### 外部与基础设施 client

- `internal/client/ai/`
  - OpenAI-compatible chat completions client
  - 同时承载普通文本 completions、multimodal completions 与 embeddings
- `internal/client/imagegen/`
  - SiliconFlow 图片生成 client
- `internal/client/bbh/`
  - BBH HTTP client
- `internal/client/http/`
  - 通用下载 bytes 的 HTTP client
- `internal/client/imagestore/`
  - 本地图片读写与访问 URL 生成
- `internal/client/pgstore/`
  - PostgreSQL 数据访问对象 `Store`

### 配置与数据

- `internal/config/`
  - `.env` 与环境变量读取
- `internal/dal/model/`
  - gorm/gen 生成的 model
- `internal/dal/query/`
  - gorm/gen 生成的 query 层，目前主业务基本未直接使用

### 通用工具

- `internal/util/`
  - `uconvert`、`uface`、`uimage`、`unapcat`、`urand`、`uslice`、`utext`、`utime`

## 启动链路

启动顺序如下：

1. `config.Load()` 读取配置
2. `pgstore.InitStore(cfg.DSN())` 初始化数据库访问对象
3. `service.NewService(...)` 装配业务依赖
4. `ws.NewServer(cfg.ListenAddr, service)` 创建 WS 服务
5. `ListenAndServe()` 开始接收 NapCat 推送

关键文件：

- `cmd/server/main.go`
- `internal/config/config.go`
- `internal/client/pgstore/pgstore.go`
- `internal/service/service.go`
- `internal/transport/ws/server.go`

## 入站消息主链路

NapCat 入站 JSON 的处理流程：

1. `internal/transport/ws/server.go`
   - 接收文本帧
   - 调用 `napcat.ParseInboundMessage`
2. `internal/napcat/parser.go`
   - 识别 `group_message` / `notice` / `action_response`
3. `internal/handler/napcat/handler.go`
   - `HandleGroupMessage`
   - `HandleNotice`
   - `HandleActionResponse`
4. `internal/service/`
   - 执行命令、消息入库、图片查重、表情记录、报告生成等业务逻辑

## 当前主要边界

### `handler/napcat`

负责：

- NapCat 入站事件接入
- 白名单/黑名单等入口判断
- 调用 `service`
- 发送文本、segments、图片/file、emoji like 动作

不负责：

- 数据库访问
- 命令具体业务
- 图片查重策略
- face / emoji like 数据编排

### `service`

负责：

- 命令匹配与命令执行
- 消息入库
- face / emoji like 业务
- 图片查重、白名单、图片生成
- 报告格式化
- 回执补写

### `client/pgstore`

负责：

- PostgreSQL 连接初始化
- `Store` 及所有数据库读写
- `StoredMessage`、`StoredImage`、`ReportStats` 等数据库查询结果类型

### `client/http`

负责：

- 基于 `http.Client` 下载远程内容为 `[]byte`

## 当前 service 包结构

`internal/service/` 当前文件大致分为以下几类：

- 命令相关
  - `commands.go`
  - `prompts.go`
  - `command_ai.go`
  - `command_bbh.go`
  - `command_generate_image.go`
  - `command_face.go`
  - `command_faceid.go`
  - `command_getfaceid.go`
  - `command_allface.go`
  - `command_json.go`
  - `command_image_to_file.go`
  - `command_dice.go`
  - `command_symmetric.go`
- 入库与表情
  - `save_message.go`
  - `save_image.go`
  - `save_face.go`
- 图片
  - `image.go`
- 业务辅助
  - `ai_fallback.go`
  - `memory.go`
  - `memory_factory.go`
  - `memory_queue.go`
  - `outbound.go`
  - `report.go`
- 装配
  - `service.go`

## 命令系统

### 命令定义位置

- `internal/service/prompts.go`
  - `commandKey`
  - `commandDefs`
  - `helpText`
  - AI 命令的 `SystemPrompt`

### 命令编译与分发

- `internal/service/service.go`
  - `NewService`
  - 编译命令正则
- `internal/service/commands.go`
  - `MatchCommand`
  - `ExecuteCommand`
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
  - `.faceid`
  - `.getfaceid`
  - `.allface`
- 工具类
  - `.生图`
  - `.XdY`
  - `.对称左` / `.对称右` / `.对称上` / `.对称下`
  - `.对称左上` / `.对称右上` / `.对称左下` / `.对称右下`
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

## 图片子系统

图片业务当前集中在 `internal/service/image.go`，但底层下载能力已迁到 `internal/client/http/http.go`。

当前行为：

- 下载图片 bytes
- 计算 pHash
- 保存到 `image` 表
- 在同群历史里做相似图查重
- 动画表情图走白名单逻辑

生成图片文件的本地存取在：

- `internal/client/imagestore/image_store.go`

静态访问在：

- `internal/transport/ws/server.go`
  - `/images/`

## 数据存取

数据库读写现在统一在：

- `internal/client/pgstore/pgstore.go`

其中包含：

- `Store`
- 用户、群、消息、表情、图片、报告等数据库方法
- 结果类型：
  - `StoredMessage`
  - `StoredImage`
  - `GetFaceIDMessageRow`
  - `ReportStats`
  - `ReportDay`
  - `ReportNight`
  - `ReportAtUser`

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
- `EMBED_API_KEY`
- `EMBED_BASE_URL`
- `EMBED_MODEL_NAME`
- `IMAGE_GEN_API_KEY`
- `IMAGE_GEN_BASE_URL`
- `IMAGE_GEN_MODEL_NAME`
- `FREE_MODEL_API_KEY`
- `FREE_MODEL_BASE_URL`
- `FREE_MODEL_NAME`
- `BBH_BASE_URL`
- `MY_URL`
- `BOT_USER_ID`
- `BOT_NICKNAME`
- `GROUP_IDS`
- `BANNED_USER_IDS`

`.env` 模板见：

- `.env.example`

## 测试布局

当前测试主要分布如下：

- `internal/service/service_test.go`
- `internal/service/command_symmetric_test.go`
- `internal/service/image_test.go`
- `internal/config/config_test.go`
- `internal/napcat/parser_test.go`
- `internal/transport/ws/server_test.go`
- `internal/client/imagestore/image_store_test.go`

## 调试方法

本仓库当前本地调试建议顺序：

1. 先跑测试
2. 再执行 `sh run.sh`
3. 服务成功启动即可

注意：

- 当前启动脚本是 `run.sh`
- 不是 `run_ws_server.sh`
- 离线记忆生产通过 `sh run.sh --memory` 启动

## 常见改动导航

### 想加一个新命令

优先看：

- `internal/service/prompts.go`
- `internal/service/commands.go`
- `internal/service/command_*.go`
- `internal/service/service_test.go`

### 想改消息保存逻辑

优先看：

- `internal/service/service_ingest.go`
- `internal/client/pgstore/pgstore.go`

### 想改系统表情/表情回应逻辑

优先看：

- `internal/service/face_storage.go`
- `internal/service/command_face.go`
- `internal/service/command_faceid.go`
- `internal/service/command_getfaceid.go`
- `internal/service/command_allface.go`
- `internal/client/pgstore/pgstore.go`
- `internal/napcat/types.go`

### 想改消息发送逻辑

优先看：

- `internal/handler/napcat/handler.go`
- `internal/handler/napcat/send.go`
- `internal/service/state.go`
- `internal/napcat/types.go`

### 想改协议解析

优先看：

- `internal/napcat/parser.go`
- `internal/napcat/types.go`
- `internal/transport/ws/server.go`

### 想改 AI 相关行为

优先看：

- `internal/service/command_ai.go`
- `internal/client/ai/ai.go`
- `internal/service/ai_fallback.go`
- `internal/service/prompts.go`

补充说明：

- 当前主模型历史消息类调用已支持多模态输入
- 当多模态失败时，会依次降级为：
  - 全部图片
  - 最新一张图片
  - 纯文本单模态

### 想改图片逻辑

优先看：

- `internal/service/image.go`
- `internal/service/command_generate_image.go`
- `internal/client/imagegen/siliconflow.go`
- `internal/client/http/http.go`
- `internal/client/pgstore/pgstore.go`
- `internal/client/imagestore/image_store.go`

## 已知注意事项

### 1. 命令消息不一定入库

不要默认当前触发命令一定能在数据库最近消息里查到。

### 2. `raw_json` 不是同一种形态

历史消息里既可能是 segments 数组，也可能是机器人自发文本的 JSON 字符串。

### 3. action 回执并不统一

`CompleteActionResult` 当前更偏向处理 `send_group_msg` 的成功回执，不要拿它套所有 NapCat action。

### 4. topic/word 相关表不是当前主业务热点

当前报告输出已经不展示“热门话题/高频词汇”。

### 5. 图片 URL 与本地生成文件 URL 不是一回事

- `image.url` 保存的是原始图片来源 URL
- `imagestore.ReadImage(...)` 返回的是本地生成文件的访问 URL

### 6. `internal/util` 现在是活跃公共层

如果遇到纯函数、跨文件复用逻辑，先看是否应该落到 `internal/util/u*`

### 7. `.生图` 直接回发远程 URL

当前 `.生图` 返回的结果图片 URL 不会先下载到本地，而是直接复用现有发图链回发。

## 推荐阅读顺序

第一次接手本仓库时，建议按这个顺序阅读：

1. `cmd/server/main.go`
2. `internal/config/config.go`
3. `internal/client/pgstore/pgstore.go`
4. `internal/service/service.go`
5. `internal/handler/napcat/handler.go`
6. `internal/service/service_ingest.go`
7. `internal/service/commands.go`
8. `internal/service/prompts.go`
9. `internal/napcat/types.go`
10. `internal/napcat/parser.go`
11. `internal/transport/ws/server.go`

## 文档维护原则

后续修改本文件时，优先保持以下原则：

- 描述当前实现，不写一次性任务计划
- 优先反映真实目录和真实调用链
- 重点写“入口、职责、调用关系、坑点”
- 对已不再存在的旧目录、旧脚本、旧命名要及时删除，不做兼容描述
