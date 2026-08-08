#!/bin/bash

# 定义变量，方便后续修改
PORT="11003"
SERVICE_NAME="njk_go"
# 如果你的可执行文件不在这个脚本所在目录，请将其改为绝对路径，例如 /home/frank/njk_go/njk_go-linux-amd64-v2.1.1
EXEC_PATH="./njk_go-linux-amd64-v2.1.1"

echo ">>> 开始重启 $SERVICE_NAME 服务..."

# 1. 查找占用端口的 PID
# lsof 的 -t 参数只返回 PID，非常适合脚本处理
PID=$(sudo lsof -i:$PORT -t)

# 2. 判断进程是否存在并终止
if [ -n "$PID" ]; then
    echo "发现旧进程 PID: $PID，正在终止..."
    sudo kill $PID
    
    # 等待 1 秒，确保端口释放
    sleep 1
    
    # 再次检查，如果进程还在（可能卡住了），则强制杀掉
    if sudo lsof -i:$PORT -t > /dev/null; then
        echo "进程未响应，正在强制终止 (kill -9)..."
        sudo kill -9 $PID
    fi
    echo "旧进程已终止。"
else
    echo "端口 $PORT 未被占用，无需终止旧进程。"
fi

# 3. 启动新服务
echo "正在启动新服务: $EXEC_PATH"

# 检查文件是否存在
if [ ! -f "$EXEC_PATH" ]; then
    echo "错误: 找不到可执行文件 $EXEC_PATH"
    exit 1
fi

# 使用 nohup 后台运行
nohup $EXEC_PATH &

echo ">>> 服务启动完成！"
