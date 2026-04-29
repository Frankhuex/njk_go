# emoji_like 任务分析与执行计划

## 范围说明

- 只关注当前 Go 仓库
- 不再参考旧 Python 仓库
- 当前阶段只做分析与计划，不写功能代码

## 协议信息

### 收到表情回复

```json
{
  "type": "face",
  "data": {
    "id": "123"
  }
}
```

结论：

- 历史消息的 `segments` 中，系统表情段的 `type` 为 `face`
- 需要读取 `data.id`
- 在当前 Go 类型里，对应 `napcat.MessageSegment{Type, Data}`，其中 `Data.ID` 的类型是 `napcat.ID`

### set_msg_emoji_like - 设置表情回复

#### 参数

| 字段名     | 数据类型 | 默认值 | 说明 |
|------------|----------|--------|------|
| message_id | number   | -      | 消息 ID |
| emoji_id   | string   | -      | 表情 ID |

#### 响应数据

无

结论：

- 需要连续发送多个 `set_msg_emoji_like`
- 每次调用的 `message_id` 都应是触发命令 `.face n` 这条指令消息本身的 ID
- 每次调用的 `emoji_id` 来自历史消息中所有 `face` 段的 `data.id`
- 不需要确认回执
- 不需要把这些动作当作“机器人发言”落库

## 根目录 Markdown 梳理结果
### napcat_api_urls.md

- 已登记“设置消息表情点赞”
- 已登记“获取群历史消息”
- 但当前 Go 代码里尚未真正封装 `set_msg_emoji_like`
- 当前任务并不需要接 NapCat 的远程群历史接口，因为本仓库已经把群消息落到本地数据库，可直接读本地历史

## 代码库现状总结

### 1. 命令系统

- 命令定义集中在 `internal/bot/prompts.go`
- 命令编译注册在 `internal/bot/service.go`
- 命令匹配与分发在 `internal/bot/commands.go`
- 新命令 `.face n` 的接入方式，应与现有命令一致：
  - 新增 `commandKey`
  - 在 `commandDefs` 中新增 regex
  - 在 `buildCommandHandler` 中挂接 handler
  - 如需对外展示，再更新 `helpText`

推荐正则：

```text
^ *\.face +(\d+) *$
```

含义：

- 必须以 `.face` 开头
- `.face` 后至少一个空格
- 再跟数字参数 `n`
- 前后空格可容忍

### 2. 入站消息落库

- `internal/bot/service_ingest.go` 会把入站消息 `event.Message.Segments` 做 `json.Marshal`
- 结果存入 `message.raw_json`
- 同时还会把 `event.RawMessage` 存入 `message.raw_message`

这意味着本任务需要的原始 `segments` 数据已经具备，不需要额外回查消息详情接口

### 3. 数据库结构

- `sql/create_njk_tables.sql` 中 `message.raw_json` 是 `JSONB`
- `internal/model/message.gen.go` / `internal/bot/store.go` 对应了消息表访问
- 当前历史查询接口 `RecentMessages` / `MessagesSince` 只返回：
  - `message_id`
  - `time`
  - `sender_id`
  - `nickname`
  - `card`
  - `text`
  - `raw_message`

缺口：

- 现在并没有把 `raw_json` 查出来
- 因而 `.face n` 不能直接复用现有 `StoredMessage` 结构，需要扩展查询结果，或新增一个专用查询函数

### 4. raw_json 的解析目标

- 入站消息保存时，`raw_json` 来自 `event.Message.Segments`
- 因而解析目标应为 `[]napcat.MessageSegment`
- 遍历后读取每个 segment 的：
  - `segment.Type`
  - `segment.Data.ID`

识别规则：

- 当 `segment.Type == "face"` 时，记录 `segment.Data.ID.String()`

### 5. 历史消息来源

- 当前 Go 仓库已有本地数据库历史查询能力
- `.ai` 等命令就是直接查本地 `message` 表
- 本任务最合适的实现也应直接从本地库读取“本群前 n 条消息”

这里的“前 n 条消息”建议解释为：

- 取该群最近的 n 条已落库消息
- 不包含当前 `.face n` 指令消息本身

原因：

- 命令匹配发生在当前事件上
- 当前事件只有在部分路径下才会先落库
- `.face n` 是一个独立命令，更自然的语义是“向前看 n 条历史消息”

### 6. NapCat 发送链路

- 当前项目已有 `send_group_msg` 的发送封装
- 发送后会进入 `pendingQueue`
- 成功回执若带 `message_id`，会触发“保存机器人自己发言”

对本任务的影响：

- `set_msg_emoji_like` 不是普通文本消息发送
- 也不应进入“机器人发言落库”这条链路
- 需要走一个不依赖 `SendMsgResponseData` 的动作发送路径
- 可以发送 action，但不要把它当作 `send_group_msg`

### 7. 现有回执处理限制

- 当前 `HandleActionResponse` 默认按 `SendMsgResponseData` 反序列化
- 它期待 `data.message_id`
- 而 `set_msg_emoji_like` 文档写的是“响应数据无”

因此：

- 本任务不能复用“发送文本消息并等待 message_id 回执”的保存逻辑
- 最简单稳妥的做法是：
  - 直接发送 `set_msg_emoji_like`
  - 不入 `pendingQueue`
  - 不依赖 action 回执做后续处理

## 任务实现所需的最少改动

### A. 命令层

- 在 `prompts.go` 新增 `commandFace`
- 在 `commandDefs` 新增 `.face n` 的 pattern
- 在 `commands.go` 的 `buildCommandHandler` 中新增分支
- 新增独立 handler，建议放在新的 `internal/bot/command_face.go`

### B. 仓储层

二选一：

1. 扩展现有 `StoredMessage`，让历史查询把 `raw_json` 一并查出
2. 新增一个专用于 `.face` 的历史查询结构，只查本任务真正需要的字段

更推荐第 2 种，因为：

- 不会影响 `.ai` / `.aic` / 报告等现有调用
- 责任更单一
- 更容易控制“是否排除当前指令消息”这类条件

建议新查询返回：

- `message_id`
- `raw_json`

可选再带：

- `time`

### C. 解析层

- 把数据库中的 `raw_json` 字符串反序列化为 `[]napcat.MessageSegment`
- 遍历所有消息段
- 收集全部 `face` 的 `data.id`
- 保持出现顺序即可，无需去重，除非后续产品语义要求“同一个表情只贴一次”

当前按原需求理解，建议：

- 不去重
- 遇到几个 `face` 就发送几次 `set_msg_emoji_like`

### D. NapCat action 层

- 在 `internal/napcat/types.go` 新增 `set_msg_emoji_like` 对应请求参数结构
- 在 bot 侧新增一个“发送通用 action”的小封装，或者新增专用 `sendMsgEmojiLike`
- 每个 `face id` 都发送一次 action：
  - `action = "set_msg_emoji_like"`
  - `params.message_id = 当前命令消息 ID`
  - `params.emoji_id = face id`

### E. 回执与存储层

- 不等待回执
- 不把 action 回执挂到 `pendingQueue`
- 不调用 `saveSelfMessage`
- 不新增 message 表记录

## 建议的实现步骤

1. 新增 `.face n` 命令定义和 handler 接入口
2. 在 store 中补一个“查询本群最近 n 条历史消息原始 raw_json”的专用方法
3. 在 handler 中读取历史消息，并排除当前指令消息
4. 逐条反序列化 `raw_json` 为 `[]napcat.MessageSegment`
5. 扫描所有 `face` 段并收集 `data.id`
6. 为 NapCat 新增 `set_msg_emoji_like` 请求结构与发送函数
7. 以当前指令消息 ID 为 `message_id`，循环发送每个 `emoji_id`
8. 发送完成后直接结束，不等待回执、不落库
9. 补单元测试，重点验证命令匹配、历史解析、face 收集与空结果分支

## 需要特别确认的实现语义

按当前需求，我会采用以下语义：

- `n` 表示读取本群最近 n 条历史消息
- 不包含当前 `.face n` 指令消息
- 从这些消息的 `raw_json` 中读取全部 `face`
- 读取顺序按历史消息顺序处理
- 同一条消息中多个 `face` 全部生效
- 不去重
- 若没有读到任何 `face`，则静默结束，不额外发送文本提示

## 风险与注意点

### 1. raw_json 兼容性

- 当前入站消息的 `raw_json` 是 `segments` 数组
- 但机器人自己发言的 `raw_json` 是一个被 JSON 编码的纯文本字符串
- 如果历史范围里包含机器人自发消息，直接按 `[]napcat.MessageSegment` 解析会失败

处理建议：

- 对空值直接跳过
- 对无法解析为 `[]napcat.MessageSegment` 的 `raw_json` 直接跳过
- 不让单条坏数据中断整个 `.face` 流程

### 2. 当前命令消息是否已入库

- `HandleGroupMessage` 中，命令消息未必会先进入保存逻辑
- 因而查询最近 n 条历史消息时，最好不要假设当前命令消息已经在数据库里
- 更安全的做法是：直接查最近 n 条已存在历史，语义上天然就是“命令之前的历史”

### 3. action 回执模型不匹配

- 现有 action 回执逻辑偏向 `send_group_msg`
- `set_msg_emoji_like` 不应复用这条存储逻辑
- 本任务最稳妥方案是单纯发送 action，不把它纳入已有 pending 保存机制

### 4. 帮助文本是否更新

- 如果希望用户可发现 `.face n`
- 需要同步更新 `helpText`
- 若暂时不想暴露在帮助里，可以先不写

## 测试计划

- 命令匹配测试：
  - `.face 5` 命中
  - `.face     12` 命中
  - `.face` 不命中
  - `.face abc` 不命中
- 仓储查询测试：
  - 能按群拿到最近 n 条消息的 `raw_json`
- 解析测试：
  - `raw_json` 为 segments 数组时能正确解析出 `face`
  - 混合 `text` / `face` / `reply` 时只收集 `face`
  - 非法 JSON 或字符串型 `raw_json` 会被安全跳过
- 发送测试：
  - 收集到 3 个 `face id` 时，发送 3 次 `set_msg_emoji_like`
  - 每次都使用当前命令消息 ID 作为 `message_id`
  - 不触发“保存自己消息”

## 结论

这个任务在当前 Go 仓库中是可落地的，核心依赖已经齐全：

- 命令系统已有
- 本地消息库已有
- `raw_json` 已保存
- NapCat action 通道已有

真正需要补的是三块：

1. `.face n` 的命令入口
2. 带 `raw_json` 的历史消息查询与 `face` 解析
3. `set_msg_emoji_like` 的专用 action 发送封装

按上述方案实现时，不需要依赖旧 Python 仓库，也不需要接入远端群历史接口。
