# 多模态 AI 输入设计

## 1. 目标

在当前项目中保留现有单模态文本聊天能力的前提下，新增一套多模态输入能力，用于将：

- 最近 `n` 条消息的文本内容
- 以及这 `n` 条消息中所有附带图片的图片 URL

一起组织成 SiliconFlow `chat/completions` 的多模态请求。

本设计只描述方案，不写代码。

---

## 2. 外部接口结论

根据 SiliconFlow `chat/completions` 文档，其多模态请求的 `messages[].content` 不是字符串，而是一个数组。

典型结构如下：

```json
{
  "model": "zai-org/GLM-4.6V",
  "messages": [
    {
      "role": "user",
      "content": [
        {
          "type": "text",
          "text": "What's in this image?"
        },
        {
          "type": "image_url",
          "image_url": {
            "url": "https://example.com/image1.jpg"
          }
        }
      ]
    }
  ]
}
```

也就是说，多模态的关键点是：

- `content` 为数组
- 文本项：
  - `{"type":"text","text":"..."}`
- 图片项：
  - `{"type":"image_url","image_url":{"url":"..."}}`

因此当前项目的 `AIClient.Complete(...)` 不能直接承担多模态请求，需要新增单独方法。

---

## 3. 当前实现现状

### 3.1 `AIClient`

当前 `internal/client/ai/ai.go` 中：

- `Complete(...)` 走单模态文本请求
- `chatMessage.Content` 是 `string`

现状适合：

- system prompt 为文本
- user prompt 为文本

不适合：

- user content 为 `[]content_part`

### 3.2 数据读取

当前 `pgstore` 已有：

- `RecentMessages(ctx, groupID, limit)`
- `RecentMessageImages(ctx, groupID, limit)`

但这两者是分离的：

- 一个返回消息列表
- 一个返回“最近 `n` 条消息中关联的图片列表”

还没有一个方法能一次性返回：

- 最近 `n` 条消息
- 以及每条消息附带的全部图片

---

## 4. 核心需求解释

用户要求新增一个：

- `RecentMessageAndImages`

它要获取：

- 最近 `n` 条消息
- 以及这些消息中 *所有* 有附带图片的消息的图片

这里的重点不是“只取一张图”，而是：

- 最近 `n` 条消息全部保留
- 这些消息里所有出现的图片都要带出来

因此最合理的数据结构不是“消息数组 + 扁平图片数组”，而是：

- 消息为主
- 每条消息下挂自己的图片列表

---

## 5. 推荐数据结构

建议在 `internal/client/pgstore/pgstore.go` 中新增一个聚合结果类型，例如：

```go
type RecentMessageAndImagesRow struct {
    Message StoredMessage
    Images  []StoredImage
}
```

或者更直接一些：

```go
type MessageWithImages struct {
    MessageID  string
    Time       time.Time
    SenderID   string
    Nickname   string
    Card       string
    Text       string
    RawMessage string
    RawJSON    string
    Images     []StoredImage
}
```

更推荐第二种，因为：

- 业务层使用更直接
- 不必再解 `StoredMessage`

但如果想减少新结构字段重复，也可以选择第一种。

---

## 6. `RecentMessageAndImages` 设计

### 6.1 推荐签名

建议在 `pgstore.Store` 中新增：

```go
func (s *Store) RecentMessageAndImages(ctx context.Context, groupID string, limit int) ([]MessageWithImages, error)
```

### 6.2 行为

输入：

- `groupID`
- `limit`

输出：

- 最近 `limit` 条消息，按时间正序排列
- 每条消息都附带该消息关联的全部图片

### 6.3 排序要求

建议返回时满足：

- 消息整体按时间正序
- 每条消息下的图片按 `image.id` 或消息内出现顺序正序

这样业务层构造多模态 prompt 时顺序稳定。

### 6.4 查询策略

推荐两段式查询：

1. 先查最近 `n` 条消息
2. 再根据这些消息的 `message_id` 批量查 `image`
3. 在 Go 内存里按 `message_id` regroup

原因：

- 实现简单
- 容易保持消息正序
- 不需要写复杂 join 聚合

---

## 7. 文本打包规则

多模态聊天仍然需要一段文本说明，因此需要把最近 `n` 条消息组织成一个 `text` content part。

### 7.1 保留全部消息文本

这里和 `.生图` 的规则不完全一样。

本需求建议：

- 最近 `n` 条消息的文本全部参与
- 因为多模态聊天通常需要完整上下文

### 7.2 是否保留发送者

当前需求没有要求去掉发送者。

建议保留简洁发言上下文：

```text
张三：今天天气好
李四：这张图怎么样
王五：我觉得还行
```

原因：

- 多模态问答时，发送者线索往往有助于模型理解对话关系

如果后续某个具体场景明确要求只留纯文本，再单独裁剪。

### 7.3 文本来源

建议优先：

- `RawMessage`

回退：

- `Text`

### 7.4 清洗规则

建议至少：

- `TrimSpace`
- 过滤空消息

是否像 `.生图` 一样去掉 `[]` 包裹内容，要看具体场景：

- 如果希望更贴近人类原始发言，可删除 `[CQ:...]`
- 如果想保留更多上下文，可暂时不删

设计上建议抽成可配置的辅助函数，而不要硬写死在 `AIClient` 里。

---

## 8. 多模态消息组装方案

### 8.1 system message

仍建议保留单独的 system message：

```json
{
  "role": "system",
  "content": "..."
}
```

它依然是纯字符串。

### 8.2 user message

新增一个多模态 user message：

```json
{
  "role": "user",
  "content": [
    {"type": "text", "text": "...最近 n 条消息整理后的文本..."},
    {"type": "image_url", "image_url": {"url": "https://..."}},
    {"type": "image_url", "image_url": {"url": "https://..."}}
  ]
}
```

### 8.3 content part 顺序

建议固定顺序：

1. 第一个 part 一定是 `text`
2. 后面按消息时间顺序追加所有图片 `image_url`

例如：

```json
[
  {"type":"text","text":"最近群聊如下：..."},
  {"type":"image_url","image_url":{"url":"图1"}},
  {"type":"image_url","image_url":{"url":"图2"}}
]
```

这样模型先看文本上下文，再看图片，更符合阅读顺序。

---

## 9. `AIClient` 设计调整

### 9.1 保留现有单模态

必须保留当前：

```go
Complete(ctx, systemPrompt, userPrompt, temperature)
```

用于现有所有文本 AI 命令，不可破坏兼容性。

### 9.2 新增多模态方法

建议新增一个新方法，例如：

```go
func (c *AIClient) CompleteMultimodal(
    ctx context.Context,
    systemPrompt string,
    text string,
    imageURLs []string,
    temperature *float64,
) (string, error)
```

这个签名最容易直接服务于当前业务。

如果想更通用，也可以新增更底层形式：

```go
func (c *AIClient) CompleteWithMessages(
    ctx context.Context,
    messages []ChatMessage,
    temperature *float64,
) (string, error)
```

但就目前项目规模来说，先上业务友好的 `CompleteMultimodal(...)` 更合适。

### 9.3 内部数据结构

建议新增新的消息结构，不要强行复用当前 `chatMessage`：

```go
type multimodalMessage struct {
    Role    string `json:"role"`
    Content any    `json:"content"`
}
```

以及：

```go
type multimodalContentPart struct {
    Type     string                  `json:"type"`
    Text     string                  `json:"text,omitempty"`
    ImageURL *multimodalImageURLPart `json:"image_url,omitempty"`
}

type multimodalImageURLPart struct {
    URL string `json:"url"`
}
```

因为：

- 单模态 `content` 是 `string`
- 多模态 `content` 是 `[]part`

两者结构不同，最好明确分开，避免大量 `any` 污染现有逻辑。

---

## 10. 推荐调用链

建议未来某个具体业务场景这样调用：

1. `pgstore.RecentMessageAndImages(...)`
2. 业务层将最近 `n` 条消息整理成一段文本
3. 提取这些消息下的全部图片 URL
4. 调用：

```go
aiClient.CompleteMultimodal(ctx, systemPrompt, text, imageURLs, temperature)
```

5. 得到文本回复

这条链与当前：

```go
aiClient.Complete(...)
```

并行存在，互不影响。

---

## 11. Service 层建议

当前阶段先设计，不绑定某一个具体命令。

但未来在 `service` 层使用时，建议抽一个帮助函数，例如：

```go
func buildMultimodalPrompt(rows []pgstore.MessageWithImages) (string, []string)
```

职责：

- 从最近 `n` 条消息提取文本上下文
- 收集全部图片 URL
- 去重空 URL

返回：

- 文本 prompt
- 图片 URL 列表

这样业务代码不需要重复处理消息和图片聚合。

---

## 12. 错误处理建议

### 12.1 没有消息

如果最近 `n` 条消息为空：

- 直接返回错误

### 12.2 没有图片

是否允许“纯文本走多模态接口”取决于具体业务。

从设计角度看：

- `CompleteMultimodal(...)` 最好允许没有图片
- 这样它可以退化成“只有 text part 的 content 数组”

但某些上层业务如果强依赖图片，可以自行提前校验。

### 12.3 只有图片没有文本

也可以允许：

- 自动补一段固定说明文本

例如：

```text
请结合以下图片进行分析
```

这样接口结构始终稳定，content 第一项总是 `text`。

---

## 13. 建议新增或修改的文件

### 修改

- `internal/client/pgstore/pgstore.go`
  - 新增 `RecentMessageAndImages(...)`
- `internal/client/ai/ai.go`
  - 保留 `Complete(...)`
  - 新增 `CompleteMultimodal(...)`

### 可选新增

- `internal/service/multimodal_helpers.go`
  - 负责消息文本与图片 URL 组装

---

## 14. 推荐结论

推荐方案是：

1. 在 `pgstore` 中新增 `RecentMessageAndImages(...)`
2. 返回“消息 + 该消息附带的全部图片”结构
3. 在 `AIClient` 中保留现有 `Complete(...)`
4. 新增 `CompleteMultimodal(...)`
5. 多模态请求统一用：
   - 一个 `system` 文本消息
   - 一个 `user` 多模态消息
   - `content = [text, image_url, image_url, ...]`

核心原则：

- 单模态与多模态并存
- 不破坏现有文本命令链
- 多模态请求结构严格遵循 SiliconFlow 文档
- 消息和图片的组织放在业务层或 pgstore 聚合层，不要把业务拼装逻辑硬塞进底层 HTTP client
