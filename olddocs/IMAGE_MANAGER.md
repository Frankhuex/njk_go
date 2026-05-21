# 图片保存与读取模块开发清单

## 1. 本次需求边界

本次要开发的是一个独立的图片存取模块，暂时**不接入现有 bot 业务逻辑处理链路**，实现以下能力：

1. `SavePNG`
   - 输入：PNG 画布、文件名
   - 规则：如果文件名已以 `.png` 或 `.PNG` 结尾，则不追加后缀；否则追加 `.png`
   - 行为：将图片编码为 PNG，保存到项目根目录 `images/` 下

2. `SaveGIF`
   - 输入：`*gif.GIF`、文件名
   - 规则：如果文件名已以 `.gif` 或 `.GIF` 结尾，则不追加后缀；否则追加 `.gif`
   - 行为：将 GIF 保存到项目根目录 `images/` 下

3. `ReadImage`
   - 输入：文件名，调用方自行传完整文件名和后缀
   - 行为：从项目根目录 `images/` 下查找文件
   - 找到时：返回 `MY_URL + "/images/" + 文件名`
   - 找不到时：返回错误

4. 图片 HTTP 服务器
   - 复用现有服务端口
   - 服务项目根目录 `images/` 下的文件
   - 至少支持通过 `/images/<文件名>` 访问

本阶段不做：

- 不接 `internal/bot/`
- 不改消息入站/出站逻辑
- 不落数据库
- 不处理 NapCat 图片消息
- 不把该能力挂到任何现有命令
- 不改现有命令处理语义

## 2. 对当前代码库的认知

### 2.1 项目当前主链路

当前仓库是 NapCat 反向 WebSocket 群机器人：

- 入口在 `cmd/server/main.go`
- 核心业务在 `internal/bot/`
- WebSocket 服务在 `internal/transport/ws/`
- 配置在 `internal/config/config.go`

现有主流程围绕“消息处理、命令执行、数据库存储、图片查重”展开。

### 2.2 当前图片相关实现

仓库里已经有“图片处理”代码，但它与这次需求不是同一层：

- `internal/bot/image.go`
  - 负责下载远程图片
  - 计算 pHash
  - 做图片查重和白名单处理
- `internal/bot/service_ingest.go`
  - 在入站消息保存时提取图片 URL
  - 调用图片查重逻辑
- `internal/bot/store.go`
  - 只保存图片 hash，不保存本地图片文件

也就是说，当前仓库的“图片能力”是**查重链路**，不是“本地图片文件管理模块”。

### 2.3 当前没有的东西

本次需求涉及的一些基础设施，仓库目前还没有（即将新增）：

- 没有项目根目录 `images/` 目录
- 没有独立的图片 client 包
- 没有 `MY_URL` 配置项
- 没有复用现有端口的 `/images/*` 静态资源服务
- 没有现成的本地图片读写封装

### 2.4 当前 client 风格

当前较活跃的 client 风格主要参考：

- `internal/ai/client.go`
- `internal/bbh/client.go`

它们的共同点：

- 都使用独立 package
- 都有 `Client` 结构体
- 都有 `NewClient(...)`
- 配置和值通过构造函数注入

另外，`internal/client/db.go` 更像旧残留，不建议把新图片模块继续塞进这个目录。

## 3. 本次需求的实现建议

### 3.1 推荐新增位置

建议新增一个独立包：

- `internal/imagestore/`

推荐原因：

- `imagestore` 更贴近本次真实职责：本地图片保存与读取
- 避免放进 `internal/client/` 与旧代码混在一起
- 避免包名直接叫 `image`，减少与 Go 标准库 `image` 包的混淆

推荐文件：

- `internal/imagestore/client.go`
- `internal/imagestore/client_test.go`

### 3.2 推荐模块形态

建议做成带配置的 `Client`，而不是三个散落的全局函数。

推荐方向：

- `Client` 持有 `imageServerURL`
- `Client` 持有图片根目录路径
- 对外提供：
  - `SavePNG(...)`
  - `SaveGIF(...)`
  - `ReadImage(...)`

这样做的好处：

- 现在不接业务，也能独立开发
- 后续若接入 bot/service，只需在装配层注入
- 测试时可以把图片目录指向临时目录，避免污染仓库根目录

### 3.3 关于 `MY_URL`

当前仓库没有 `MY_URL` 配置项。

根据新的约束，`MY_URL` 应作为 `.env` 环境变量接入当前配置系统。

当前 `internal/config/config.go` 的读取方式是：

- 先读环境变量
- 环境变量为空时，再看 `.env`
- 两者都没有时，使用 fallback

因此，本次需求后续开发时建议同步调整：

- `internal/config/config.go`
  - 在 `Config` 中新增 `MyURL string`
  - 在 `Load()` 中读取 `MY_URL`
  - fallback 使用 `http://localhost:11003`
  - 与 `BASE_URL`、`BBH_BASE_URL` 一样做 `strings.TrimRight(..., "/")`
- `.env.example`
  - 新增 `MY_URL="http://localhost:11003"`

另外，根据你的约束，链接是否真的可访问不由代码端到端保证：

- `ReadImage` 只负责按规则拼接 `MY_URL + "/images/" + 文件名`
- 访问可用性由配置者保证
- 图片 HTTP 服务器只保证按本地目录提供静态文件

## 4. 需求细节拆解

### 4.1 `SavePNG`

需明确的行为：

- 输入类型使用 `image.Image`
- 需要注意：Go 标准库里的 `image.Image` 是接口，不是结构体
- 文件名如果没有 `.png` / `.PNG` 后缀，则自动补 `.png`
- 保存前确保 `images/` 目录存在；若不存在，应自动创建
- 使用标准库 PNG 编码
- 默认保存到项目根目录下的 `images/`

推荐补充约束：

- 文件名主体只接受大小写字母、数字、下划线
- 禁止文件名包含路径穿越内容，例如 `../`
- 禁止文件名带目录分隔符，避免越界写入到 `images/` 外
- 允许覆盖同名文件

我的建议：

- 允许覆盖同名文件
- 文件名主体仅允许大小写字母、数字、下划线
- 但必须禁止路径穿越和目录分隔符
- `SavePNG` 直接收 `image.Image`
- 不采用 `*image.Image`

### 4.2 `SaveGIF`

需明确的行为：

- 输入类型使用 `*gif.GIF`
- 原因：标准库 `gif.EncodeAll` 直接接收 `*gif.GIF`
- 文件名如果没有 `.gif` / `.GIF` 后缀，则自动补 `.gif`
- 保存前确保 `images/` 目录存在
- 使用标准库 GIF 编码保存

待开发者确认点：

- 无

我的建议：

- 使用 `*gif.GIF`
- 这样与标准库编码函数最贴近，额外拷贝更少

### 4.3 `ReadImage`

需明确的行为：

- 入参文件名由调用方自行提供，并自行带后缀
- 只检查 `images/` 目录下是否存在该文件
- 找到时返回 URL 字符串
- 找不到时返回错误

这里要特别说明：

- `ReadImage` 返回的是“按规则拼出的访问地址”
- 该地址要真正可访问，还依赖图片 HTTP 服务已启动
- 路径规则应与 `/images/<文件名>` 保持一致

推荐补充约束：

- 同样禁止传入带路径穿越的文件名
- 文件名主体只接受大小写字母、数字、下划线，并由调用方自带合法后缀
- 返回 URL 前，对 base URL 做去尾 `/` 处理
- 生成 URL 时优先按 URL 路径拼接，而不是按系统文件路径拼接

待开发者确认点：

- 无

我的建议：

- `ReadImage` 只负责拼出链接
- 链接可访问性由配置者保证
- 由于 `MY_URL` 有默认值，正常情况下不会缺失
- 但如果最终配置为空字符串，仍建议返回错误，而不是返回残缺链接

### 4.4 图片 HTTP 服务器

需明确的行为：

- 复用现有 `WS_ADDR` 对应的 `http.Server`
- 暴露项目根目录 `images/` 目录
- 至少支持 `GET /images/<文件名>`
- 与现有 WebSocket 服务共用同一个端口
- 保持现有 WebSocket 路径不变

推荐实现方向：

- 基于标准库 `net/http`
- 使用 `http.FileServer` 或等价实现暴露 `images/`
- 在现有 `internal/transport/ws/server.go` 的 `ServeMux` 上新增 `/images/` 路由
- `cmd/server/main.go` 无需再启动第二个 HTTP 监听器

待开发者确认点：

- 无

我的建议：

- WebSocket 继续使用 `/`
- 图片静态资源使用 `/images/`
- 两者共用 `WS_ADDR`

## 5. 与现有代码的关系判断

### 5.1 本阶段无需修改的目录

这次需求不需要改这些地方：

- `internal/bot/`
- `internal/napcat/`
- `internal/model/`
- `internal/query/`
- `sql/`

原因：

- 本次不接消息处理业务逻辑
- 不接消息链路
- 不接数据库
- 不改现有图片查重系统

### 5.2 可能需要新增或调整的文件/目录

本次最小改动建议如下：

- 新增 `internal/imagestore/client.go`
- 新增 `internal/imagestore/client_test.go`
- 新增或运行时自动创建项目根目录 `images/`
- 更新 `.gitignore`
- 修改 `internal/transport/ws/server.go`，在同一个 `ServeMux` 上挂 `/images/`
- 修改 `internal/transport/ws/server_test.go`，补充共享端口下的静态资源测试

关于 `.gitignore`：

- `images/` 属于运行产物，应加入 `.gitignore`

### 5.3 本阶段建议修改的配置文件

本阶段建议直接改：

- `internal/config/config.go`
- `.env.example`

原因：

- 你已经明确要求 `MY_URL` 通过 `.env` 传入
- 当前仓库已有成熟的配置读取方式，适合沿用
- 这属于独立模块的必要配置，不算业务对接
- `MY_URL` 需要作为对外公布的访问地址使用

## 6. 推荐任务清单

按最小实现范围，后续开发可按以下顺序进行：

1. 新建 `internal/imagestore/` 包
2. 定义 `Client` 结构体和构造函数
3. 在 `Client` 中保存：
   - 图片目录路径
   - `myURL`
4. 扩展配置读取
   - 在 `internal/config/config.go` 新增 `MyURL`
   - 在 `Load()` 中读取 `MY_URL`
   - fallback 为 `http://localhost:11003`
   - 在 `.env.example` 中补充示例项
5. 实现统一的文件名规范化逻辑
   - PNG 后缀处理
   - GIF 后缀处理
   - 路径安全校验
   - 文件名主体仅允许大小写字母、数字、下划线
6. 实现统一的图片目录确保逻辑
   - 目录不存在则自动创建
7. 实现 `SavePNG`
   - PNG 画布编码为 PNG
   - 保存到 `images/`
8. 实现 `SaveGIF`
   - `*gif.GIF` 编码为 GIF
   - 保存到 `images/`
9. 实现 `ReadImage`
   - 校验文件名
   - 检查 `images/` 下文件是否存在
   - 拼接并返回图片 URL
10. 编写单元测试
   - 文件名带后缀/不带后缀
   - 大写后缀兼容
   - 自动创建目录
   - 文件存在与不存在
   - 非法文件名
   - 默认 `MY_URL`
11. 实现图片 HTTP 服务器
   - 复用现有端口
   - 暴露 `/images/` 静态目录
   - 保持 WebSocket 路由不变
12. 编写图片 HTTP 服务测试
   - `/images/<存在文件>` 返回 200
   - `/images/<不存在文件>` 返回 404
13. 修改 `internal/transport/ws/server.go`
   - 注册 `/images/` 路由
   - 保留 `/` WebSocket handler
14. 更新 `.gitignore`
   - 加入 `images/`

## 7. 推荐测试清单

建议至少覆盖以下测试场景：

- `SavePNG("a")` 最终生成 `a.png`
- `SavePNG("a.png")` 不重复追加
- `SavePNG("a.PNG")` 不重复追加
- `SaveGIF("b")` 最终生成 `b.gif`
- `SaveGIF("b.gif")` 不重复追加
- `SaveGIF("b.GIF")` 不重复追加
- `images/` 不存在时可自动创建
- 文件名包含空格、中文、短横线时会被拒绝
- 非法文件名被拒绝
- `ReadImage` 在文件不存在时返回错误
- `ReadImage` 在文件存在时返回正确 URL
- `ReadImage` 在默认 `MY_URL` 下返回正确链接
- `config.Load()` 能正确读取 `MY_URL`
- 图片 HTTP 服务在现有端口可访问 `/images/<文件名>`

推荐测试实现方式：

- 使用临时目录代替真实项目根目录 `images/`
- 通过构造函数注入测试目录
- 不直接污染仓库根目录

## 8. 关键风险与注意事项

### 8.1 项目根目录定位

“保存到项目根目录 `images/`”这句话在实现时需要落地成具体策略。

可选方案：

- 方案 A：默认基于当前工作目录创建 `images/`
- 方案 B：由 `Client` 显式接收一个图片目录路径

我的建议：

- 对外仍可表达为“项目根目录 `images/`”
- 但代码实现上最好由 `Client` 显式接收 `baseDir`
- 这样测试和后续接入都更稳

### 8.2 链接可访问性

当前 `ReadImage` 负责“确认本地文件存在并返回 URL 字符串”。

但要注意：

- 本次新增图片 HTTP 服务器后，链接可访问依赖：
- `MY_URL` 配置正确
- 现有服务已启动
- 访问路径与 `/images/<文件名>` 对齐

### 8.3 文件名大小写

需求中仅要求：

- `.png` / `.PNG` 视为已有 PNG 后缀
- `.gif` / `.GIF` 视为已有 GIF 后缀

建议实现时做大小写不敏感判断，但保留原始文件名大小写，不强制改名。

### 8.4 文件名约束

根据最新约束，文件名主体只接受：

- 大写字母
- 小写字母
- 数字
- 下划线

这意味着：

- 不接受空格
- 不接受中文
- 不接受短横线
- 不接受额外目录层级

推荐做法：

- 先把后缀和文件名主体拆开
- 只校验文件名主体
- 后缀只允许本函数负责的目标后缀

### 8.5 URL 拼接

返回链接时需要注意：

- `imageServerURL` 末尾可能带 `/`
- `MY_URL` 末尾可能带 `/`
- 文件名里可能有空格或其他需要 URL 编码的字符

建议开发时显式处理：

- base URL 去尾 `/`
- 文件名作为 URL path segment 安全拼接

## 9. 本阶段建议的最终交付

后续开发者完成本需求时，建议交付内容为：

- 一个独立可复用的 `internal/imagestore` 包
- 三个对外方法：
  - `SavePNG`
  - `SaveGIF`
  - `ReadImage`
- 对应单元测试
- `internal/config/config.go` 对 `MY_URL` 的读取
- `.env.example` 中的 `MY_URL` 示例项
- `.gitignore` 中加入 `images/`
- 图片 HTTP 服务（复用现有端口、`/images/` 静态目录）

本阶段完成标准应限定为：

- 能正确把 PNG/GIF 写入 `images/`
- 能按规则补后缀
- 能校验本地文件是否存在
- 能生成图片链接
- 能在现有端口提供 `images/` 静态文件访问
- 不接入现有 bot 业务
- 不扩展数据库
