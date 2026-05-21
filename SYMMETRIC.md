# `.对称左` 指令开发计划

## 1. 需求摘要

新增命令：

- 正则：`^ *\.对称左 *(\d+) *$`
- 示例：`.对称左 10`

语义：

- 从当前群最近给定数量的消息中取图片
- 使用图片表中的 `url` 下载原图
- 判断图片类型
- 如果是 GIF，则按动图处理，读成 `gif.GIF`
- 如果是 JPG / JPEG / PNG 等单图，则读成 `image.Image`
- 对图片做“左半保留、右半由左半镜像生成”的处理
- 处理后调用 `imagestore.Client`
  - 单图用 `SavePNG`
  - 动图用 `SaveGIF`
- 文件名格式：`messageid_imageid.后缀`
  - `messageid` 取 `Image.MessageID`
  - `imageid` 取 `Image.ID`
  - 中间用下划线连接

本次文档只做调研与开发计划，不写代码。

## 2. 对当前代码库的认知

### 2.1 命令系统入口

当前新增命令的标准落点仍然是：

- `internal/bot/prompts.go`
  - 注册 `commandKey`
  - 注册正则
  - 如有需要，补 `helpText`
- `internal/bot/commands.go`
  - 在 `buildCommandHandler` 里挂 handler
- `internal/bot/command_xxx.go`
  - 新建独立 handler 文件实现实际逻辑
- `internal/bot/service_test.go`
  - 补命令匹配和核心行为测试

这和当前 `.face`、`.2d6` 的落点一致。

### 2.2 图片数据来源

当前最新相关链路如下：

- `internal/bot/store.go`
  - 已有 `RecentMessageImages(ctx, groupID, limit) ([]model.Image, error)`
  - 语义是：
    - 先取该群最近 `limit` 条消息
    - 再根据这些 `message_id` 查询 `image`
- `internal/model/image.gen.go`
  - `Image` 目前已有：
    - `ID`
    - `MessageID`
    - `ImageHash`
    - `URL *string`

因此：

- 本需求不需要再从 `raw_json` 里解析图片段
- 可以直接用 `RecentMessageImages(...)` 读取图片记录
- 需要对 `URL == nil` 或空串的图片做跳过处理

### 2.3 当前图片处理能力

现有图片相关逻辑主要在：

- `internal/bot/image.go`

已有能力：

- 下载远程图片
- 用 `image.Decode` 解析静态图片
- 计算 pHash
- GIF/JPEG/PNG 解码器已经通过匿名导入注册

但现状里没有：

- 按 MIME / 扩展名区分单图和 GIF 的通用工具
- GIF 的 `gif.DecodeAll`
- 左右镜像图像处理逻辑
- 对称图命令的 handler

### 2.4 当前 imagestore 能力

当前 `internal/imagestore/client.go` 已实现：

- `SavePNG(image.Image, fileName)`
- `SaveGIF(*gif.GIF, fileName)`
- `ReadImage(fileName)`

并具备以下约束：

- 文件名主体仅允许大小写字母、数字、下划线
- 可带合法后缀
- 自动保存到项目根目录 `images/`
- 支持覆盖同名文件

对本需求来说：

- `messageid_imageid` 这一命名格式与当前规则兼容
- 最终文件名应写成：
  - `messageid_imageid.png`
  - `messageid_imageid.gif`
- 保存完成后可以直接通过 `ReadImage(...)` 拿到可发送的图片 URL

### 2.5 当前 Service 结构

`internal/bot/service.go` 里的 `Service` 当前已有：

- `store`
- `imageService`
- AI / BBH client

但还没有：

- `imagestore.Client` 字段

所以如果新命令要保存处理后的图片，后续开发时需要：

- 在 `Service` 中新增 `imagestoreClient`
- 在 `NewService(...)` 中构造并注入

### 2.6 当前图片发送能力

当前 `internal/bot/service_ingress.go` 已有：

- `sendGroupImage(...)`
- `multiSendGroupImages(...)`

其中：

- `multiSendGroupImages(...)` 可以一次发送多张图片
- 底层走 `send_group_msg` 的分段消息
- 当前 `ShouldSave=false`

因此本需求的输出方式已经明确：

- 对称处理完成并保存后
- 调 `imagestore.ReadImage(...)` 取回图片 URL
- 最后使用 `multiSendGroupImages(...)` 统一回复到群里

## 3. 需求实现拆解

### 3.1 命令匹配

需要新增命令：

- `commandSymmetricLeft`

需要新增正则：

- `^ *\.对称左 *(\d+) *$`

正则含义：

- 点号前允许前导空格
- 命令关键字固定是 `对称左`
- 后面一个整数表示读取的“最近消息数”

建议：

- 该命令和 `.face` 类似，直接走独立 handler 文件
- 文件名建议：
  - `internal/bot/command_symmetric.go`

### 3.2 读取图片记录

建议 handler 流程：

1. 解析参数 `count`
2. 调 `s.store.RecentMessageImages(ctx, groupID, count)`
3. 如果结果为空：
   - 返回“历史不足”或“最近消息中没有图片”
4. 遍历这些 `model.Image`
5. 对 `URL == nil` 的记录直接跳过

需要注意：

- `RecentMessageImages` 的返回量可能大于 `count`
- 因为“最近 `count` 条消息”中每条消息可以含多张图

### 3.3 图片下载

本需求需要把库中的 `url` 下载成二进制。

当前项目已有下载逻辑：

- `ImageService.download(ctx, url string) ([]byte, error)`

但它是 `image.go` 内部私有方法。

后续开发有两个可选方向：

- 方案 A：把下载逻辑抽成共享 helper
- 方案 B：在新命令文件里复写一个最小下载函数

我的建议：

- 采用方案 A
- 把下载能力提到更容易复用的位置
- 避免 bot 内出现第二份 HTTP 下载实现

### 3.4 类型判断

用户要求：

- “注意类型别名和大小写都兼容”

因此类型判断不能只写死一种扩展名。

建议识别顺序：

1. 先看响应头 `Content-Type`
2. 再看 URL 扩展名
3. 再用文件头 / 解码尝试兜底

建议支持的静态图别名：

- `jpg`
- `jpeg`
- `png`

建议支持的 GIF 别名：

- `gif`

建议大小写处理：

- 统一转小写后比较

推荐策略：

- 若判定为 GIF，走 `gif.DecodeAll`
- 否则按静态图走 `image.Decode`

这样能兼容：

- `.JPG`
- `.JPEG`
- `.PNG`
- `.GIF`
- `image/jpeg`
- `image/jpg`
- `image/png`
- `image/gif`

### 3.5 单图处理

对单图的目标效果是：

- 中间竖着对半分开
- 左半保持不变
- 右半由左半镜像生成

建议实现方式：

- 先解码为 `image.Image`
- 新建一张目标图
- 左半拷贝原图
- 右半按 `dstX -> srcX` 镜像映射

建议处理规则：

- 新图宽高保持不变
- 统一按“左半保留，右半由左半镜像填充”处理
- 奇数宽和偶数宽不需要分成两套语义讨论

实现时可以直接按目标像素位置映射来源像素，保证：

- 宽度不变
- 高度不变
- 右半完全由左半镜像生成

### 3.6 GIF 处理

GIF 需要逐帧处理。

建议流程：

1. `gif.DecodeAll`
2. 遍历 `gif.GIF.Image`
3. 对每一帧做与单图相同的“左保留右镜像”
4. 保留原 GIF 的：
   - `Delay`
   - `LoopCount`
   - `Disposal`
   - `Config`
5. 生成新的 `gif.GIF`
6. 调 `imagestore.SaveGIF`

建议注意：

- 每一帧通常是 `*image.Paletted`
- 直接处理时需要考虑调色板保留
- 如果实现复杂，也可以先转成统一画布再回写，但这样更容易带来颜色偏差

推荐优先方案：

- 尽量保留 `Paletted` 与原调色板

### 3.7 文件命名与保存

文件名规则已经明确：

- `messageid_imageid.后缀`

建议映射：

- 单图统一保存为 PNG
  - 文件名：`messageid_imageid.png`
- GIF 保存为 GIF
  - 文件名：`messageid_imageid.gif`

这样最稳定：

- 静态图不用保留原格式，统一 PNG 便于输出
- GIF 动图语义保持不变

由于当前 `imagestore` 已限制文件名主体字符，所以这里需要确认：

- `messageID` 是否总是只含数字或其他合法字符

按当前 NapCat / QQ 消息 ID 使用习惯，通常可以通过，但开发时仍建议加保护：

- 如果 `messageID` 含非法字符，需先做规范化或跳过

## 4. 推荐改动文件

### 4.1 必改

- `internal/bot/prompts.go`
  - 新增 `commandKey`
  - 新增正则注册
  - 如需展示，补充 `helpText`
- `internal/bot/commands.go`
  - 新增 handler 分发
- `internal/bot/service.go`
  - 给 `Service` 注入 `imagestore.Client`
- `internal/bot/command_symmetric.go`
  - 新建，对称左命令主体

### 4.2 很可能要改

- `internal/bot/image.go`
  - 抽公共下载逻辑，供新命令复用
- `internal/bot/store.go`
  - 如果要补排序或附带更多图片信息，可能继续调整
- `internal/bot/service_test.go`
  - 补命令匹配测试
- 新增图片处理测试文件
  - 例如 `internal/bot/command_symmetric_test.go`

### 4.3 大概率不用改

- `internal/imagestore/client.go`
  - 当前保存接口已足够
- `internal/transport/ws/server.go`
  - 图片静态访问能力已具备
- `sql/`
  - 当前表结构已包含 `image.url`

## 5. 推荐实现步骤

1. 在 `prompts.go` 注册 `.对称左`
2. 在 `commands.go` 挂 handler
3. 在 `service.go` 注入 `imagestore.Client`
4. 新建 `command_symmetric.go`
5. 在 handler 中解析 `count`
6. 调 `RecentMessageImages(ctx, groupID, count)`
7. 过滤掉没有 `url` 的图片
8. 下载图片二进制
9. 判定是 GIF 还是静态图
10. 静态图走“左半保留右半镜像”
11. GIF 逐帧做同样处理
12. 生成 `messageid_imageid.png/gif`
13. 调 `imagestore.SavePNG` 或 `SaveGIF`
14. 通过 `imagestore.ReadImage(...)` 收集已保存图片的 URL
15. 调 `multiSendGroupImages(...)` 批量回复处理结果
16. 补单元测试与最小集成测试

## 6. 推荐测试清单

### 6.1 命令匹配

- `.对称左 5` 可以匹配
- `.对称左5` 也需要匹配
- 前后空格兼容

### 6.2 类型识别

- `jpg`
- `jpeg`
- `png`
- `gif`
- 大小写变体
- MIME 和扩展名不一致时的处理策略

### 6.3 单图对称

- 偶数宽图片镜像正确
- 奇数宽图片镜像正确
- 高度保持不变

### 6.4 GIF 对称

- 多帧 GIF 每帧都被处理
- `Delay` / `LoopCount` 保持
- 输出仍可被正常解码

### 6.5 存储

- 文件名符合 `messageid_imageid.后缀`
- 能成功保存到 `images/`
- 可通过 `ReadImage` 读取 URL

## 7. 风险点

### 7.1 输出数量可能很多

当前需求已经明确：

- 处理完成后需要回复
- 回复方式是调用 `multiSendGroupImages(...)`

因此这里的风险不再是“是否回复”，而是：

- 最近 `count` 条消息中可能提取出很多张图
- 一次性回发很多图片可能导致单条消息过长，或触发平台侧限制

开发时建议预留保护：

- 例如限制单次最多发送多少张
- 或者分批发送

### 7.2 最近消息数与实际图片数不是一一对应

命令参数是“最近多少条消息”，不是“最近多少张图”。

因此：

- 可能 10 条消息里没有图
- 也可能 10 条消息里含很多张图

实现和用户预期要保持一致。

### 7.3 GIF 颜色与调色板

GIF 的逐帧处理比静态图更容易出现：

- 颜色偏差
- 帧错位
- 透明区域异常

开发时需要重点验证。

### 7.4 下载失败和脏数据

数据库里可能存在：

- `url = nil`
- `url` 已失效
- 类型与扩展名不一致
- 图片内容损坏

建议：

- 单张失败不影响其他图片继续处理
- 对失败项做日志记录

## 8. 待确认问题

当前这两个点已经明确：

1. `.对称左5` 这种不带空格的写法需要支持
2. 奇数宽与偶数宽无需拆开定义，新图宽高保持不变即可

## 9. 建议结论

这条需求在当前仓库上是可落地的，且基础设施已经具备了大半：

- 命令系统已有成熟入口
- 最近图片读取函数已经存在
- 图片 URL 已经入库
- `imagestore` 已经能保存 PNG / GIF
- `/images/` 静态访问也已具备

真正需要新增的核心是三块：

- 命令注册与 handler
- 图片类型识别与下载复用
- 左右对称图像处理逻辑

当前输出路径已经明确：

- 保存到 `images/`
- 用 `imagestore.ReadImage(...)` 生成 URL
- 用 `multiSendGroupImages(...)` 批量回发到群里
