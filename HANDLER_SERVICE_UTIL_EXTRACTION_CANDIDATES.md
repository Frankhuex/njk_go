# handler / service 通用工具函数抽离梳理

## 目标

本文档用于梳理当前 `internal/handler/napcat` 与 `internal/service` 中，哪些函数或代码片段适合继续抽离到 `internal/util/` 下。

本次梳理遵循以下判断标准：

1. 能否剥离对 `*Service` 的依赖。
2. 能否改造成“明确传参、明确返回”的纯函数或近纯函数。
3. 是否已经出现跨文件、跨层复用，或存在重复实现。
4. 是否属于通用技术逻辑，而不是强业务语义。

本次只做调研与文档整理，不涉及代码改造。

## 总体结论

当前最值得先抽到 `internal/util/` 的，是以下三大类：

1. 文本与字符串规范化工具
2. 图片扩展名 / 类型识别工具
3. NapCat 消息内容解析与 face ID 提取工具

其次值得抽离的是：

1. 提及机器人判断工具
2. 随机区间与随机 sleep 工具
3. 结构体转 CQ 参数串工具

不建议立即抽到 `internal/util/` 的，主要是：

1. 明显带业务语义的出站构造函数
2. 明显带命令领域语义的格式化逻辑
3. 虽然是纯函数，但目前复用收益仍然很低的局部辅助函数

## 一览表

| 候选项 | 当前文件位置 | 当前用途 | 是否适合抽离 | 建议落点 | 说明 |
| --- | --- | --- | --- | --- | --- |
| `normalizeOutboundText` | `internal/handler/napcat/send.go` | 统一文本换行与转义规范 | 是 | `internal/util/utext` | 纯字符串处理，无状态，且未来很可能被多处发送链路复用 |
| `normalizedImageExt` | `internal/handler/napcat/send.go`、`internal/service/command_symmetric.go` | 从 URL/路径推断图片扩展名 | 是 | `internal/util/uimage` | 已出现重复实现，适合统一 |
| `fallbackImageExt` | `internal/handler/napcat/send.go` | 根据字节流与 URL 回退图片扩展名 | 是 | `internal/util/uimage` | 明确输入输出，可纯函数化 |
| `fileSegmentNameFromImageData` | `internal/handler/napcat/send.go` | 基于图片数据生成 file segment 文件名 | 是 | `internal/util/uimage` | 与图片类型识别同域，纯函数 |
| `fallbackFileSegmentName` | `internal/handler/napcat/send.go` | 无法识别时回退文件名 | 是 | `internal/util/uimage` 或 `internal/util/ufile` | 纯函数，和文件名生成逻辑应聚合 |
| `faceIDsFromSegments` | `internal/service/face_storage.go` | 从消息段提取系统表情 ID | 是 | `internal/util/unapcat` 或 `internal/util/uface` | 纯解析逻辑，复用价值高 |
| `emojiLikeFaceIDs` | `internal/service/face_storage.go` | 从 notice likes 提取表情 ID | 是 | `internal/util/unapcat` 或 `internal/util/uface` | 纯解析逻辑 |
| `sortFaceIDs` | `internal/service/face_storage.go` | face ID 的数字友好排序 | 是 | `internal/util/uface` | 明确输入输出，且被多处 face 逻辑共享 |
| `extractFaceIDsFromRawJSON` | `internal/service/command_face.go` | 从历史 `raw_json` 中解析 face ID | 是 | `internal/util/unapcat` 或 `internal/util/uface` | 纯解析逻辑，适合和 `faceIDsFromSegments` 放一起 |
| `mentionsBot` | `internal/service/service_ingest.go` | 判断消息是否 @bot | 是 | `internal/util/unapcat` | 只依赖消息内容和 bot id |
| `randomRange` | `internal/service/helpers.go` | 生成区间随机数 | 是 | `internal/util/urand` | 已基本是 util 形态 |
| `SleepRandomMillis` | `internal/service/helpers.go` | 带 context 的随机等待 | 是 | `internal/util/urand` | 需要把 RNG 显式作为参数，避免 Service 依赖 |
| `StructToKeyValue` | `internal/service/helpers.go` | 结构体转 CQ 参数串 | 是 | `internal/util/ucodec` | 可以抽，但建议顺手做稳定排序 |
| `emptyToNil` | `internal/service/service_ingest.go` | 空字符串转 `*string` | 可抽但优先级低 | `internal/util/uptr` | 足够纯，但收益较低 |
| `compactStringsInOrder` | `internal/service/command_symmetric.go` | 保序去重字符串 | 可抽但优先级低 | `internal/util/utext` | 纯函数，但当前复用点少 |
| `sanitizeSymmetricToken` | `internal/service/command_symmetric.go` | 过滤文件名 token | 可抽但优先级低 | `internal/util/ufile` 或 `internal/util/utext` | 纯函数，但仍偏命令局部逻辑 |
| `imageToFileSourceURLsFromRecords` | `internal/service/command_image_to_file.go` | 从图片记录中提取非空 URL | 可抽但需重塑 | `internal/util/ucollection` | 不建议直接携带 `model.Image` 依赖搬走 |

## 详细分析

### 1. 高优先级候选

这些候选同时满足以下至少两个条件：

- 已经跨层复用
- 已经出现重复实现
- 明显是纯技术逻辑
- 抽离后可以直接降低 handler/service 耦合

### 1.1 文本规范化

候选函数：

- `normalizeOutboundText`

当前位置：

- [send.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/handler/napcat/send.go#L146-L152)

适合抽离的原因：

1. 这是纯字符串转换逻辑，不依赖 `Service`、DB、client 或上下文状态。
2. 它是典型发送链路公共能力，不应挂在 handler 文件里。
3. 后续如果还有别的发送实现，例如 HTTP、测试桩、别的协议适配层，也会需要同样逻辑。

建议改造方式：

- 抽为 `utext.NormalizeOutboundText(text string) string`

建议落点：

- `internal/util/utext`

### 1.2 图片扩展名与文件名生成

候选函数：

- `normalizedImageExt`
- `fallbackImageExt`
- `fileSegmentNameFromImageData`
- `fallbackFileSegmentName`

当前位置：

- [send.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/handler/napcat/send.go#L116-L160)
- [command_symmetric.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/command_symmetric.go#L188-L196)

适合抽离的原因：

1. 当前已经存在重复的 `normalizedImageExt` 实现语义。
2. 这组代码都属于“根据 URL 或图片字节流推断扩展名/文件名”的纯技术逻辑。
3. 这些函数不应该附着在 handler 或对称命令文件中，否则未来新图片功能还会继续复制。

建议改造方式：

- `uimage.NormalizedExt(sourceURL string) string`
- `uimage.FallbackExt(sourceURL string, data []byte) string`
- `uimage.FileSegmentNameFromData(index int, sourceURL string, data []byte) string`
- `uimage.FallbackFileSegmentName(index int, sourceURL string) string`

建议落点：

- `internal/util/uimage`

### 1.3 face ID 与 NapCat 消息解析

候选函数：

- `faceIDsFromSegments`
- `emojiLikeFaceIDs`
- `sortFaceIDs`
- `extractFaceIDsFromRawJSON`

当前位置：

- [face_storage.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/face_storage.go#L13-L70)
- [command_face.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/command_face.go#L40-L50)

适合抽离的原因：

1. 它们都是纯解析或纯排序逻辑。
2. 这组代码已经跨命令处理、notice 处理、数据库结果整理等多个链路使用。
3. 它们天然依赖的是 `internal/napcat` 的消息定义，而不是 `Service` 本身。

建议改造方式：

- `uface.FaceIDsFromSegments(segments []napcat.MessageSegment) []string`
- `uface.EmojiLikeFaceIDs(likes []napcat.EmojiLike) []string`
- `uface.SortFaceIDs(faceIDs []string)`
- `uface.ExtractFaceIDsFromRawJSON(raw string) ([]string, error)`

建议落点：

- 如果希望强调 NapCat 消息结构：`internal/util/unapcat`
- 如果希望强调系统表情子域：`internal/util/uface`

建议优先选 `uface`，因为这组逻辑虽然基于 NapCat 消息结构，但业务意图其实是系统表情处理。

## 2. 中优先级候选

这些函数适合抽，但需要顺手处理一点边界问题，或者当前复用程度稍弱于第一批。

### 2.1 判断是否 @bot

候选函数：

- `mentionsBot`

当前位置：

- [service_ingest.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/service_ingest.go#L153-L159)

为什么适合抽：

1. 它不依赖 `Service` 状态，只依赖 `message` 与 `botUserID`。
2. 这是协议内容判定逻辑，不该藏在入库文件里。
3. handler 与 service 未来都可能复用。

建议改造方式：

- `unapcat.MentionsUser(message napcat.MessagePayload, userID string) bool`

建议落点：

- `internal/util/unapcat`

额外建议：

- 抽离时不要命名成 `MentionsBot`，改成更通用的 `MentionsUser` 更合理。

### 2.2 随机区间与随机 sleep

候选函数：

- `randomRange`
- `SleepRandomMillis`

当前位置：

- [helpers.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/helpers.go#L59-L80)

为什么适合抽：

1. 函数本身已经是 util 风格。
2. handler 发送表情动作时也已经在复用。
3. 这类基础随机逻辑不应该继续挂在 service helpers 中。

需要注意的问题：

1. 当前调用方仍然通过 `s.rng` 或 `h.service.Random()` 传随机源。
2. 仓库现有文档已提示共享 `rand.Rand` 可能存在并发风险。
3. 因此抽离时应把 RNG 边界一起做清楚，而不是只做文件搬家。

建议改造方式：

- `urand.Range(rng *rand.Rand, left int, right int) int`
- `urand.SleepMillis(ctx context.Context, rng *rand.Rand, left int, right int) error`

建议落点：

- `internal/util/urand`

### 2.3 结构体转 CQ 参数串

候选函数：

- `StructToKeyValue`

当前位置：

- [helpers.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/helpers.go#L102-L131)

为什么适合抽：

1. 本质是通用编码工具。
2. 不依赖 `Service`，输入输出清晰。
3. 未来任何 CQ 兼容字符串拼装都可能复用。

需要注意的问题：

1. 当前 map 遍历存在非稳定顺序。
2. 如果抽到 util，最好顺手把 key 排序稳定化。

建议改造方式：

- `ucodec.StructToKeyValue(v any) (string, error)`

建议落点：

- `internal/util/ucodec`

## 3. 低优先级候选

这些函数虽然能抽，但现阶段收益不大，建议排在后面。

### 3.1 `emptyToNil`

当前位置：

- [service_ingest.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/service_ingest.go#L162-L167)

判断：

- 非常纯，确实可以抽。
- 但目前复用点太少，不建议为了“看起来更 util”而单独建子包。

更合适的时机：

- 当仓库里出现更多“值转指针”的构造逻辑时再统一处理。

### 3.2 `compactStringsInOrder`

当前位置：

- [command_symmetric.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/command_symmetric.go#L298-L306)

判断：

- 纯函数，可抽。
- 但目前高度局部化，抽离收益有限。

### 3.3 `sanitizeSymmetricToken`

当前位置：

- [command_symmetric.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/command_symmetric.go#L202-L213)

判断：

- 纯函数，可抽。
- 但当前强绑定对称图片命令的文件名构造语境。

### 3.4 `imageToFileSourceURLsFromRecords`

当前位置：

- [command_image_to_file.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/command_image_to_file.go#L33-L45)

判断：

- 从逻辑上看是“过滤记录里的非空 URL”，是可抽的。
- 但它现在依赖 `model.Image`，如果直接搬进 util，会让 `internal/util` 依赖 DAL model，不够干净。

更合理的处理方式：

- 先重塑成更通用的“从集合提取非空字符串字段”的工具，或者继续留在命令文件里。

## 4. 暂不建议抽离的内容

### 4.1 出站动作构造函数

包括：

- `simpleOutbound`
- `imageOutbound`
- `fileOutbound`
- `segmentsOutbound`
- `savedReplyOutbound`

当前位置：

- [helpers.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/helpers.go#L22-L48)

不建议抽离原因：

1. 它们承载的是本项目自身的出站动作语义。
2. 这类逻辑属于业务层辅助，不属于通用 util。
3. 抽到 `internal/util` 会让 util 变成“项目业务胶水层”。

### 4.2 业务文本格式化

包括：

- `.getfaceid` 的输出格式化
- `.allface` 的输出格式化
- 报告输出格式化

相关位置：

- [command_getfaceid.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/command_getfaceid.go)
- [command_allface.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/command_allface.go)
- [report.go](file:///Users/bytedance/Documents/njk_proj/njk_go/internal/service/report.go)

不建议抽离原因：

1. 这些都直接编码了业务输出规则。
2. 放进 util 会模糊“通用工具”和“业务格式器”的边界。

### 4.3 命令领域强语义算法

例如：

- `.对称` 的镜像算法
- `.bbh` 的段落预览裁剪

不建议抽离原因：

1. 虽然部分函数是纯函数，但语义非常强。
2. 它们更适合留在对应命令域附近，而不是进入通用 util。

## 5. 推荐的 util 子包规划

如果下一阶段开始真正抽离，建议优先采用下面这组结构：

```text
internal/util/
├── utext/
│   └── normalize.go
├── uimage/
│   ├── ext.go
│   └── filename.go
├── uface/
│   ├── parse.go
│   └── sort.go
├── unapcat/
│   └── mention.go
├── urand/
│   └── rand.go
├── ucodec/
│   └── kv.go
└── uptr/
    └── ptr.go
```

说明：

- `uface` 用于放系统表情相关纯处理逻辑。
- `unapcat` 只放真正通用的 NapCat 内容判定逻辑，例如“是否提及某用户”。
- `uimage` 放图片扩展名、图片种类、文件名生成等纯技术处理。
- `ucodec` 放结构化对象到字符串表示的编码辅助。

## 6. 推荐迁移顺序

### 第一批

优先建议先抽：

1. `normalizeOutboundText`
2. `normalizedImageExt` 相关函数
3. face ID 工具组

原因：

- 纯度最高
- 复用最明确
- 风险最低
- 最容易立即减少重复实现

### 第二批

其次建议抽：

1. `mentionsBot`
2. `randomRange` / `SleepRandomMillis`
3. `StructToKeyValue`

原因：

- 也适合抽，但会牵涉命名、RNG 边界或稳定性细节

### 第三批

最后再考虑：

1. `emptyToNil`
2. `compactStringsInOrder`
3. `sanitizeSymmetricToken`
4. `imageToFileSourceURLsFromRecords`

原因：

- 纯度没问题，但当前收益偏小

## 7. 最终建议

站在当前代码状态看，`internal/util/` 最应该承接的是“可复用的纯技术逻辑”，而不是“看起来短小的所有函数”。

因此建议按以下原则执行：

1. 只抽纯逻辑，不抽业务语义。
2. 只抽重复点或跨层复用点，不抽一次性局部辅助。
3. 抽离时同时修正边界问题，例如 RNG 来源和 key 顺序稳定性。
4. 第一批优先从文本、图片、face 解析这三类开始。

如果后续要进入代码落地阶段，建议先从 `utext`、`uimage`、`uface` 三个子包开始实施。
