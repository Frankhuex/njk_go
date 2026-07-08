# NJK Go 使用说明

## 项目简介

这是一个基于 NapCat 反向 WebSocket 的群聊机器人服务，当前支持：

- 群消息、notice、action 回执处理
- PostgreSQL 消息落库
- AI 回复
- BBH 命令
- 图片查重与图片生成
- 系统表情相关命令

当前程序入口是项目根目录 `main.go`。

## 当前目录结构

核心目录如下：

- `internal/service/`
  - 业务编排核心
- `internal/handler/napcat/`
  - NapCat 事件入口与发送执行
- `internal/client/pgstore/`
  - PostgreSQL `Store`
- `internal/client/ai/`
  - AI client
- `internal/client/bbh/`
  - BBH client
- `internal/client/http/`
  - 下载 bytes 的 HTTP client
- `internal/client/imagestore/`
  - 本地图片读写与图片 URL 生成
- `internal/napcat/`
  - NapCat 协议定义与入站解析
- `internal/transport/ws/`
  - WebSocket server 与 `/images/` 静态资源

## 环境准备

### 1. PostgreSQL

请先准备一个可访问的 PostgreSQL 数据库，并提前创建项目所需表结构。

### 2. 环境变量

在项目根目录创建 `.env`，参考 `.env.example`。

当前主要配置项包括：

- 数据库相关
  - `DB_NAME`
  - `DB_HOST`
  - `DB_USER`
  - `DB_PWD`
  - `DB_PORT`
- AI 相关
  - `API_KEY`
  - `BASE_URL`
  - `MODEL_NAME`
  - `FREE_MODEL_NAME`
- 服务相关
  - `WS_ADDR`
  - `MY_URL`
  - `BOT_USER_ID`
  - `BOT_NICKNAME`
  - `GROUP_IDS`
  - `BANNED_USER_IDS`
  - `BBH_BASE_URL`

## 启动 Go 服务

### 直接运行

```bash
sh run.sh
```

`run.sh` 会自动寻找本机可用的 Go 可执行文件，并执行：

```bash
go run .
```

### 手动运行

如果本机 Go 环境已配置好，也可以直接执行：

```bash
go run .
```

## NapCat 配置

### 1. 启动 NapCat 容器

根据你的部署方式启动 NapCat，例如：

```bash
docker compose -p 项目名称 up --build -d
```

注意：

- `ports` 左侧是宿主机端口，按实际需要修改
- `container_name` 不能与本机已有容器重复

### 2. 登录 NapCat

查看容器日志：

```bash
docker logs -f 容器名称
```

然后：

- 扫码登录
- 记录 token

### 3. 配置 WebSocket 客户端

进入 NapCat Web UI 后，创建 WebSocket 客户端：

- URL 填写：

```text
ws://host.docker.internal:你的Go服务端口
```

其中端口应与你配置的 `WS_ADDR` 保持一致。

可按需设置重连间隔，例如 `3000ms`。

## 本地图片访问

服务启动后会同时暴露：

- `/images/`

用于访问本地生成的图片文件。

图片文件由：

- `internal/client/imagestore/`

负责保存到项目根目录下的 `images/` 目录，并生成可访问 URL。

## 开发与调试

建议本地调试顺序：

1. 先跑测试

```bash
go test ./...
```

2. 再启动服务

```bash
sh run.sh
```

## 当前启动链路

当前启动流程如下：

1. `config.Load()`
2. `pgstore.InitStore(cfg.DSN())`
3. `service.NewService(...)`
4. `ws.NewServer(...)`
5. `ListenAndServe()`

## 补充说明

- 当前数据库访问统一由 `internal/client/pgstore/pgstore.go` 提供
- 当前 NapCat 事件由 `internal/handler/napcat/handler.go` 承接
- 当前图片下载能力由 `internal/client/http/http.go` 提供
- 当前通用纯函数优先沉淀到 `internal/util/u*`
