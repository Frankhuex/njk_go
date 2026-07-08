# `.生图 n` 指令设计

## 1. 目标

新增一个图片生成能力，命令形式为：

```text
.生图 n
```

含义：

- 读取当前群最近 `n` 条已保存消息
- 从这 `n` 条消息里取“所有可用消息文本”
- 从这 `n` 条消息里取“最新的一张图片 url”
- 文本和图片两者只要有一个存在即可发起生图
- 将拿到的文本和/或图片 url 一起提交给 SiliconFlow 生图接口
- 使用模型：

```text
Kwai-Kolors/Kolors
```

- 将生成结果回发到群里

本文档只描述实现方案，不写代码。

---

## 2. 外部接口结论

根据 SiliconFlow 图片生成接口文档：

- 接口为 `POST /v1/images/generations`
- 认证方式：

```text
Authorization: Bearer <token>
```

- `Kwai-Kolors/Kolors` 主要相关请求字段：
  - `model`
  - `prompt`
  - `image_size`
  - `batch_size`
  - `num_inference_steps`
  - `guidance_scale`
  - `image`

对于本需求，最适合走文档里的 `I2Image` 方式：

- `model = "Kwai-Kolors/Kolors"`
- `prompt = 最近 n 条消息清洗后的文本拼接结果`
- `image = 最近 n 条消息里最新图片的 url`

也就是：

- 如果有文本，则作为 `prompt`
- 如果有图片，则作为 `image`
- 两者只要至少存在一个即可发起请求

这样最符合“将消息和 url 都输给生图模型”的要求。

---

## 3. 命令行为定义

### 3.1 指令格式

```text
.生图 n
```

其中：

- `n` 为正整数
- 表示取“当前群最近 `n` 条消息”

建议正则形式：

```text
^ *\.生图 *(\d+) *$
```

### 3.2 基本流程

收到 `.生图 n` 后：

1. 校验 `n`
2. 查询当前群最近 `n` 条消息
3. 查询这最近 `n` 条消息对应的图片记录
4. 选择其中“最新的一张图片 url”
5. 提取并拼接其中“所有可用消息文本”
6. 分别对文本和图片做存在性判断
7. 调用 SiliconFlow `images/generations`
8. 取返回图片 url
9. 直接复用现有发图链回发到群

---

## 4. 消息文本提取与清洗规则

这是本功能最关键的规则之一。

### 4.1 不保留发送者

用户要求：

- 打包的消息不需要保留发送者
- 只保留消息原文拼接

因此最终 prompt 文本中：

- 不要包含群昵称
- 不要包含 user_id
- 不要包含 “某某说：”
- 不要包含时间

只保留消息内容本身。

### 4.2 优先使用 `raw_message`

当前库里 `StoredMessage` 同时有：

- `Text`
- `RawMessage`

建议优先使用：

- `RawMessage`

原因：

- 这是更接近用户原始发言的字段
- 更符合“消息原文拼接”的要求

当 `RawMessage` 为空时，再回退到：

- `Text`

### 4.3 清洗规则

用户要求：

- 去掉所有半角中括号 `[]` 及其包裹的内容

因此建议对每条消息执行：

1. 去掉所有形如：

```text
[任意内容]
```

的片段

例如：

- `[CQ:image,file=xxx]`
- `[图片]`
- `[回复]`

都整体删除

推荐正则语义：

```text
\[[^\[\]]*\]
```

这版先采用“单层中括号删除”即可，不需要做复杂嵌套解析。

2. 删除后再做：
  - `strings.TrimSpace`
  - 连续空白折叠

3. 清洗后为空的消息直接丢弃

### 4.4 提取规则

用户更新后的要求是：

- 最近消息和最近消息中的最新图片只要有一个就可以

因此文本侧应当：

- 从最近 `n` 条消息中提取所有清洗后非空的消息
- 按时间正序拼接成最终 `prompt`

建议拼接方式使用换行。

### 4.5 空文本处理

如果最近 `n` 条消息全部清洗后为空：

- 不直接报错
- 只要还能找到图片，就仍然继续发起生图请求

如果文本和图片都找不到，才返回失败提示。

---

## 5. 最近图片选择规则

### 5.1 图片来源

当前项目已经有：

- `RecentMessageImages(ctx, groupID, limit)`

该方法会从“最近 `limit` 条消息”对应的图片中取出图片记录。

因此 `.生图 n` 可以直接复用这条链。

### 5.2 选择规则

用户要求：

- 获取前 `n` 条消息对应的 image 的 url 中最新的一个

因此建议：

- 从 `RecentMessageImages(...)` 返回结果中，取最后一张
- 也就是时间上最新的一张图片

如果没有图片：

- 不直接报错
- 只要还能找到文本，就仍然继续发起生图请求

### 5.3 只取一个图片 url

第一版设计只传一张：

- 最新图片 url

不做多图拼接，不做历史多图融合。

---

## 6. SiliconFlow 生图客户端设计

建议在 `internal/client/ai/` 之外单独新增一个面向图片生成的 client。

推荐位置：

- `internal/client/siliconflow/`

例如：

- `internal/client/siliconflow/image.go`

原因：

- 图片生成与聊天 completions / embeddings 不是同一种协议
- 放在单独 client 中边界更清楚

### 6.1 建议配置项

建议新增配置：

- `IMAGE_API_KEY`
- `IMAGE_BASE_URL`
- `IMAGE_MODEL_NAME`

建议默认值：

- `IMAGE_BASE_URL = https://api.siliconflow.cn/v1`
- `IMAGE_MODEL_NAME = Kwai-Kolors/Kolors`

### 6.2 请求体

建议第一版固定这些参数：

```json
{
  "model": "Kwai-Kolors/Kolors",
  "prompt": "<最近 n 条消息清洗后的文本拼接结果>",
  "image": "<最新图片url>",
  "image_size": "1024x1024",
  "batch_size": 1,
  "num_inference_steps": 20,
  "guidance_scale": 7.5
}
```

注意：

- `prompt` 和 `image` 至少要有一个存在
- 如果只有文本，则走纯文生图
- 如果只有图片，则走带参考图生图
- 如果两者都有，则同时传入

### 6.3 返回体

根据文档，主要关注：

```json
{
  "images": [
    {
      "url": "..."
    }
  ]
}
```

因此客户端只需提取：

- 第一张图片的 `url`

如果 `images` 为空：

- 返回错误

### 6.4 URL 时效性

文档明确说明：

- 生成 url 有效期只有一小时

当前第一版可以先直接回发远程 url

但要在设计上注明：

- 后续如果要增强稳定性，建议下载后再走本地 `imagestore` 保存

---

## 7. Service 层实现建议

建议新增文件：

- `internal/service/command_generate_image.go`

### 7.1 主要方法

建议新增：

- `handleGenerateImageCommand(ctx, groupID, match)`

职责：

1. 解析 `n`
2. 读取最近 `n` 条消息
3. 读取最近 `n` 条消息中的图片
4. 取最新图片 url
5. 提取全部可用消息文本并清洗拼接
6. 调用 SiliconFlow 图片生成 client
7. 返回 `imageOutbound(...)`

### 7.2 可复用能力

当前仓库已经有可复用部分：

- `RecentMessages(...)`
- `RecentMessageImages(...)`
- `imageOutbound(...)`

因此第一版只需要新增：

- 文本清洗函数
- 图片生成 client
- `.生图` 的命令处理函数

---

## 8. 命令系统接入点

需要修改：

- `internal/service/prompts.go`
- `internal/service/commands.go`
- `internal/service/service.go`

### 8.1 `prompts.go`

新增：

- `commandGenerateImage`

以及命令定义：

```text
^ *\.生图 *(\d+) *$
```

并把 `.生图` 加入 `helpText`

说明建议：

```text
.生图 后面接数字，表示结合本群最近 n 条消息，并取这 n 条消息里最新的一张图作为参考图进行生图
```

### 8.2 `commands.go`

在 `buildCommandHandler(...)` 中增加：

- `commandGenerateImage -> s.handleGenerateImageCommand(...)`

---

## 9. 文本清洗函数建议

建议新增一个纯函数，位置可选：

- `internal/service/command_generate_image.go`
- 或 `internal/util/utext/`

推荐命名：

- `sanitizeImagePromptText(raw string) string`

规则：

1. 删除所有半角中括号包裹内容
2. `TrimSpace`
3. 折叠多余空白

建议再补一个构造函数：

- `buildImagePromptFromMessages(messages []pgstore.StoredMessage) string`

职责：

- 对最近 `n` 条消息逐条清洗
- 过滤空结果
- 按时间正序拼接成最终 prompt

---

## 10. 返回方式

建议回发方式：

- `imageOutbound(groupID, []string{generatedURL})`

原因：

- 当前项目已有统一图片回发链
- 生成 url 不需要下载
- 直接用现有发图函数发出即可

### 10.1 是否保存机器人消息

第一版建议：

- `ShouldSave = false`

原因：

- 生图结果属于工具输出
- 与普通文本聊天不同，不急着纳入记忆或最近消息主链

如果后续希望保留这类图像生成结果，再单独讨论是否保存。

---

## 11. 失败场景设计

建议统一错误提示。

### 11.1 参数错误

```text
参数错误
```

### 11.2 最近消息不足

可继续复用：

```text
历史消息不足
```

### 11.3 没有可用文本

如果还有图片：

- 继续生图，不报错

如果文本和图片都没有：

```text
最近消息里没有可用于生图的内容
```

### 11.4 没有图片

如果还有文本：

- 继续生图，不报错

如果文本和图片都没有：

```text
最近消息里没有可用于生图的内容
```

### 11.5 生图接口失败

建议对用户返回简短提示：

```text
生图失败，请稍后再试
```

同时服务日志记录完整错误。

---

## 12. 日志建议

建议新增以下日志：

- `【生图命令开始】group=%s n=%d`
- `【生图文本选中】group=%s n=%d text_len=%d`
- `【生图参考图选中】group=%s image_url=%s`
- `【生图请求开始】group=%s model=%s`
- `【生图请求成功】group=%s output_url=%s`
- `【生图请求失败】group=%s err=%v`

---

## 13. 建议新增配置

在 `internal/config/config.go` 中建议新增：

- `ImageAPIKey`
- `ImageBaseURL`
- `ImageModelName`

对应环境变量：

- `IMAGE_API_KEY`
- `IMAGE_BASE_URL`
- `IMAGE_MODEL_NAME`

建议默认：

- `IMAGE_BASE_URL=https://api.siliconflow.cn/v1`
- `IMAGE_MODEL_NAME=Kwai-Kolors/Kolors`

---

## 14. 第一版实现范围

第一版只做：

- `.生图 n`
- 最近 `n` 条消息中所有可用文本的提取与清洗拼接
- 最近 `n` 条消息中最新图片 url 选取
- 调 SiliconFlow `Kwai-Kolors/Kolors`
- 直接复用现有发图链回发生成结果图片

第一版不做：

- 多图输入
- 提示词二次润色
- 先用 LLM 把聊天整理成专业绘图 prompt
- 生成结果落本地文件
- 生成结果入库
- 生图参数自定义

---

## 15. 推荐落地文件

### 新增

- `internal/client/siliconflow/image.go`
- `internal/service/command_generate_image.go`

### 修改

- `internal/config/config.go`
- `internal/service/prompts.go`
- `internal/service/commands.go`
- `cmd/server/main.go`
- `cmd/memory-factory/main.go`（如果希望 memory factory 也统一装配）
- `README.md`
- `AGENTS.md`

---

## 16. 推荐结论

这个需求和当前仓库结构是兼容的，而且落地成本不高。

最推荐的实现路径是：

1. 复用 `RecentMessages(...)`
2. 复用 `RecentMessageImages(...)`
3. 对消息做“去掉半角中括号及其内容”的清洗
4. 将最近 `n` 条消息中所有可用文本拼成 `prompt`
5. 取最近图片 url 作为 `image`
6. 调用 SiliconFlow `Kwai-Kolors/Kolors`
7. 用 `imageOutbound(...)` 回发

核心原则：

- 不保留发送者
- 只保留清洗后的消息原文
- 文本和图片任一存在即可继续
- 图片只取最近一张
- 生成 url 不下载，直接复用现有发图链
- 第一版先直连生图接口，不增加额外 LLM prompt 改写层
