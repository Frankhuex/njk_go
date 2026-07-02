# `.faceid` 指令执行方案

## 背景确认

当前项目是一个 Go WebSocket 服务器，NapCat 作为 WebSocket 客户端连入后，服务端通过同一条连接接收入站事件并发送 NapCat action。

关键链路如下：

1. `cmd/server/main.go` 装配配置、数据库、`bot.Service` 和 WebSocket server。
2. `internal/transport/ws/server.go` 负责 WebSocket 握手、读写 text frame，并把入站 payload 交给 `napcat.ParseInboundMessage`。
3. `internal/napcat/parser.go` 区分群消息、notice 和 action response。
4. `internal/bot/service_ingress.go` 的 `HandleGroupMessage` 是群消息入口，会先匹配正则命令，再分发到对应 command handler，最后统一发送出站响应。

README 只描述了基础启动方式，部分配置和功能说明已经少于当前代码。后续实现应以 `AGENTS.md` 和最新代码为准。

## 现有命令系统

命令定义集中在 `internal/bot/prompts.go`：

1. `commandKey` 常量定义命令标识。
2. `commandDefs` 返回命令正则列表。
3. `helpText` 维护 `.help` 输出。

命令编译在 `internal/bot/service.go`：

1. `NewService` 遍历 `commandDefs`。
2. 每个正则用 `regexp.MustCompile` 编译。
3. `buildCommandHandler` 为每个 key 绑定 handler。

命令匹配在 `internal/bot/commands.go`：

1. `matchCommand` 从 `s.commands` 后往前遍历。
2. 后定义的正则优先级更高。
3. 新增同前缀命令时要注意定义顺序和冲突。

当前 `.face` 正则是 `^ *\.face *(\d+) *$`，允许 `.face12`。`.faceid` 不会被它匹配，因为 `.face` 后面的 ` *` 只能匹配空格，后续要求直接是数字。

## 需求语义

新增命令：

1. `.faceid n`
2. `.faceid n-m`

出站消息应构造 NapCat segment 数组：

```json
[
  {
    "type": "face",
    "data": {
      "id": "n"
    }
  }
]
```

单个数字只发一个 segment。范围 `n-m` 按闭区间 `[n,m]` 遍历，每个整数构造一个 segment，`data.id` 使用数字的十进制字符串形式。

## 推荐设计

采用一个命令 key 和一个正则处理单值与范围：

```go
Pattern: `^ *\.faceid *(\d+)(?:-(\d+))? *$`
```

原因：

1. 单值和范围的业务完全相同，都是构造 `face` segments。
2. 一个 handler 内根据第二个捕获组是否为空判断范围，改动最小。
3. 不需要依赖命令倒序匹配优先级来区分 `.faceid n` 与 `.faceid n-m`。

如果更想贴近 `.bbh` 的分拆方式，也可以定义 `commandFaceID` 和 `commandFaceIDRange` 两个 key，并把 range 正则放在 `commandDefs` 中更靠后的位置。但这个需求没有必要拆成两个 handler。

## 数据结构调整

在 `internal/napcat/types.go` 中补充 segment 枚举：

```go
SegmentTypeFace SegmentType = "face"
```

建议同时新增构造函数：

```go
func NewFaceSegment(id ID) MessageSegment
```

这样 `.faceid` handler、已有 `.face` 测试和未来其他 face 相关逻辑都可以复用，避免散落字符串 `"face"`。

`MessageSegmentData.ID` 已经是 `napcat.ID`，它的 JSON 序列化底层是 string，因此 `Data: MessageSegmentData{ID: napcat.ID(strconv.Itoa(n))}` 会输出 `"id":"n"`，符合需求。

## 出站通道调整

当前 `pendingOutbound` 只支持这些出站形态：

1. `Message` 文本。
2. `ImageURLs` 加 `ImageSegmentType`，由 `multiSendGroupImages` 转为 image/file segments。
3. `EmojiLikeIDs`，由 `setMsgEmojiLike` 发送 `set_msg_emoji_like` action。

`.faceid` 需要发送通用 `[]napcat.MessageSegment`，不应复用 `ImageURLs`，也不应走 `set_msg_emoji_like`。

建议在 `internal/bot/state.go` 的 `pendingOutbound` 新增字段：

```go
Segments []napcat.MessageSegment
```

然后在 `internal/bot/service_ingress.go` 新增通用发送函数：

```go
func (s *Service) multiSendSegments(ctx context.Context, conn outboundWriter, groupID string, segments []napcat.MessageSegment) error
```

函数行为建议与 `multiSendGroupImages` 保持一致：

1. 空 segments 直接返回 nil。
2. 构造 `napcat.SendGroupMsgRequest{Action: "send_group_msg"}`。
3. `Params.Message` 使用 `napcat.NewSegmentMessage(segments...)`。
4. `conn.WriteText(data)` 发送。
5. `pending.Push(pendingMessage{GroupID: groupID, Message: "", SentAt: time.Now(), ShouldSave: false})`，保持与图片/文件消息一致的 action response 消费模型。
6. 日志建议写 `【发送群消息段】group=%s segment_count=%d`。

`HandleGroupMessage` 统一发送响应时，在处理 `ImageURLs` 和 `EmojiLikeIDs` 之间或之后增加：

```go
if len(response.Segments) > 0 {
    if err := s.multiSendSegments(ctx, conn, response.GroupID, response.Segments); err != nil {
        log.Printf("【发送消息段响应失败】%s - %v", clientAddr, err)
    }
}
```

这个通用通道后续也能支持 share、record、video 等非文本非图片消息段。

## Handler 设计

新增文件建议为 `internal/bot/command_faceid.go`。

handler 签名：

```go
func (s *Service) handleFaceIDCommand(ctx context.Context, groupID string, match matchedCommand) (*pendingOutbound, error)
```

`ctx` 当前不一定使用，但保留签名风格与其他 handler 一致。

解析逻辑：

1. 校验 `len(match.Groups) >= 2`。
2. `left, err := strconv.Atoi(match.Groups[1])`。
3. 如果 `match.Groups[2] == ""`，则 `right = left`。
4. 否则 `right, err := strconv.Atoi(match.Groups[2])`。
5. `left <= 0`、`right <= 0`、`left > right` 返回 `simpleOutbound(groupID, "参数错误")`。
6. 遍历 `i := left; i <= right; i++` 构造 `napcat.NewFaceSegment(napcat.ID(strconv.Itoa(i)))`。
7. 返回 `&pendingOutbound{GroupID: groupID, Segments: segments, ShouldSave: false}`。

建议加上范围上限，避免用户误发超大范围导致构造大量 segment 或触发 NapCat 限制。可以参考骰子命令的 `count > 20`，将 `.faceid` 范围长度限制为最多 20 个。超限返回文案可用 `太多啦，最多20个`。

## 具体改动清单

需要修改的文件：

1. `internal/napcat/types.go`：新增 `SegmentTypeFace` 和 `NewFaceSegment`。
2. `internal/bot/prompts.go`：新增 `commandFaceID`，在 `commandDefs` 中加入 `.faceid` 正则，并更新 `helpText`。
3. `internal/bot/commands.go`：在 `buildCommandHandler` 中接入 `commandFaceID`。
4. `internal/bot/state.go`：在 `pendingOutbound` 中新增 `Segments []napcat.MessageSegment`。
5. `internal/bot/service_ingress.go`：新增 `multiSendSegments`，并在统一发送响应处处理 `response.Segments`。
6. `internal/bot/helpers.go`：可选新增 `segmentsOutbound(groupID string, segments []napcat.MessageSegment)`，让 handler 返回值更统一。
7. `internal/bot/command_faceid.go`：新增 `.faceid` 参数解析和 segment 构造逻辑。
8. `internal/bot/service_test.go`：补命令匹配、handler 行为和发送 payload 测试。

## 测试方案

建议新增或扩展以下单测：

1. `TestMatchCommandSupportsFaceIDSingle`：`.faceid12` 和 `.faceid 12` 都匹配 `commandFaceID`，捕获组为 `12`。
2. `TestMatchCommandSupportsFaceIDRange`：`.faceid 12-15` 匹配 `commandFaceID`，捕获组为 `12` 和 `15`。
3. `TestMatchCommandRejectsInvalidFaceID`：`.faceid abc`、`.faceid 3-1`、`.faceid 0` 不产生有效发送结果或返回参数错误。
4. `TestHandleFaceIDCommandBuildsSingleFaceSegment`：handler 返回一个 `Segments`，类型为 `napcat.SegmentTypeFace`，`Data.ID == "12"`，`ShouldSave == false`。
5. `TestHandleFaceIDCommandBuildsRangeFaceSegments`：`.faceid 12-14` 返回三个 segment，ID 依次为 `12`、`13`、`14`。
6. `TestHandleFaceIDCommandRejectsLargeRange`：超过上限时返回 `太多啦，最多20个`。
7. `TestMultiSendSegmentsSendsSegmentPayload`：通过现有 `net.Pipe`/`wsTestConn` 测试发送出的 `send_group_msg` payload 中 `message` 是 segment 数组，包含 `type:"face"` 和字符串 id。

执行验证命令：

```bash
go test ./...
```

如果按仓库约定做完整本地验证，再执行：

```bash
sh run_ws_server.sh
```

服务成功启动后即可停止，真实 NapCat 联调留给人工验证。

## 注意事项

1. `.face` 是从历史消息提取 face id 并调用 `set_msg_emoji_like`，`.faceid` 是直接发送 `send_group_msg` 的 `face` segment，两者不要复用 action 通道。
2. `HandleActionResponse` 会消费 `pendingQueue`，所以 `multiSendSegments` 如果走 `send_group_msg`，最好像 `multiSendGroupImages` 一样 push 一个 `ShouldSave=false` 的 pending，避免后续回执错位。
3. 新命令属于显式命令，按当前 `HandleGroupMessage` 逻辑不会先保存触发消息，这与 `.face`、`.json`、`.file` 等命令一致。
4. 不要把 `face` segment 放进 `ImageURLs`，因为 `multiSendGroupImages` 会写 `data.file`，而需求要求 `data.id`。
5. 如果 NapCat 对单条消息中的 face segment 数量有限制，上限应优先保守，建议先用 20。
