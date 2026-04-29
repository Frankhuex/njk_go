# NJK使用方法
## 0. 安装PostgreSQL
## 1. 启动Go WebSocket服务器（就是这里的代码）
### 1.1 修改环境变量
在根目录创建.env，参考.env.example，根据实际情况定义以下8个变量：
PostgreSQL相关的：
DB_NAME
DB_HOST
DB_USER
DB_PWD
DB_PORT

大模型API相关的：
API_KEY
BASE_URL
MODEL_NAME

### 1.2 启动Go Websocket服务器
直接用Go runtime运行
```bash
sh run_ws_server.sh
```
或先编译
```bash
sh build_ws_server.sh
```
再运行
```bash
./build/njk_go-linux-amd64
```

## 2. 启动Napcat的Docker容器
### 2.1 根据实际情况修改docker-compose.yml
修改ports和container_name！
ports左侧是宿主机端口，右侧是容器端口，只需要改左侧成你要的端口。
container_name不能和电脑上其他容器名称重复。
### 2.2 启动容器
```bash
docker compose -p 项目名称 up --build -d
```
项目名称可自选，或者“-p 项目名称”如果不写就会默认用当前目录文件夹名称作为项目名称
注意新版docker compose没有短杠-了
### 2.3 进入容器日志扫码登录
```bash
docker logs -f 容器名称
```
进去找到二维码，扫码登录
再找到token，一般在二维码前面一点，记住！
按Ctrl+C退出日志
### 2.4 去WebUI配置WebSocket
浏览器登录http://你的ip:你的端口，这里端口是你在docker-compose.yml中配置的，映射到6099的端口。
输入token登录。
找到网络配置，创建WebSocket客户端，URL填：“ws://host.docker.internal:你的Go服务器端口”，端口必须和在internal/config/config.go定义的一致.

下面两个重连间隔可以修改，我喜欢3000ms
然后就能启动了