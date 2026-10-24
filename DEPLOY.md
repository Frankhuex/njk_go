# 一键部署

本地配置为 `deploy.conf`（已忽略），模板为 `deploy.conf.example`。SSH 使用 `~/.ssh/lxx_server`，目标为 `frank@129.211.13.226:22`。使用已登记的主机密钥。

```bash
bash deploy.sh 2.3
```

脚本先检查连接和依赖，再执行 `build.sh` 编译服务器架构的 Linux 程序。服务器加锁分配版本，首次从 2.3.2 推进到 2.3.3；新前缀从 .0 开始。构建失败不分配编号；分配后即使上传失败也保留编号。版本记录 `versions.log` 独立于版本目录，请勿删除。

本地将 `njk_go`、`manifest`、`SHA256SUMS` 打成 `release.tar.xz`（xz -6），连同压缩包 SHA-256 校验文件通过 rsync 上传。传输时关闭 rsync 二次压缩，显示压缩包的已传字节、百分比、速度和剩余时间，结束后显示统计。上传目标为 `releases/<版本号>.uploading/`；服务器先校验压缩包，再解包并校验内部文件，删除压缩包后重命名为正式版本目录，最后调用 `restart.sh <版本号>`。manifest 记录提交、工作区改动状态、目标架构和构建时间。

本地依赖 tar、xz，服务器使用 Python 3.12 的 tarfile/lzma 解包；正式版本目录仍只保留程序、manifest 和 SHA256SUMS。

辅助命令：

```bash
bash deploy.sh 2.3 --check
bash deploy.sh 2.3 --upload-only
```

`--check` 只检查连接、架构和目录，不构建或分配编号。`--upload-only` 完成构建、版本分配、上传及校验，保留当前运行程序。

## 服务器目录和进程

部署根目录为 `/home/frank/njk`。`.env` 保留在此目录，权限为 600。`images`、`runtime` 链接至旧目录 `/home/frank/njk_go` 下对应数据目录，`temp` 单独保留。旧目录仍存放运行数据，请勿删除。

`releases/<版本号>/` 保存独立程序版本，`current` 指向当前版本。已保存原始 2.3.2 程序作为回退版本。版本暂不自动清理。

`restart.sh` 为入口，实际控制逻辑为 `scripts/server/deploy_server.py`，使用 Python 3 标准库。用户级 systemd `njk.service` 管理应用，工作目录固定为部署根目录，通过 `current/njk_go` 启动。已启用开机启动和用户 linger，退出 SSH 后继续运行。

发布任务自身也交给临时 systemd 单元执行，SSH 中断后继续完成。发布、版本分配共用 `deploy.lock`。服务器验证校验值、版本顺序后原子切换 current 并重启，连续检查进程版本和 WebSocket HTTP 426 响应。启动失败则恢复旧链接并重启旧版本，返回非零状态。

部署流程不执行数据库初始化、迁移或备份，不上传 SQL 和建表脚本。

## 服务器操作

以下命令在服务器执行：

```bash
systemctl --user status njk.service
journalctl --user -u njk.service -f
cat /home/frank/njk/deploy-history.log
bash /home/frank/njk/restart.sh 2.3.2 --rollback
bash /home/frank/njk/restart.sh --recover
```

`--rollback` 仅允许回到曾成功运行过的版本；历史成功最高版本仍保留，后续版本号继续递增。`--recover` 用于发布进程被强制终止后恢复记录中的旧版本。普通 SSH 断开由临时 systemd 发布任务继续处理。

## 安装或更新控制脚本

本地上传控制脚本：

```bash
rsync -az --relative -e 'ssh -i ~/.ssh/lxx_server -o IdentitiesOnly=yes' restart.sh scripts/server/deploy_server.py frank@129.211.13.226:/home/frank/njk/
```

首次安装时，在服务器执行 `bash /home/frank/njk/restart.sh --install`，登记旧版并创建用户级 systemd 服务。此次已完成安装，后续更新控制脚本只需上传，勿重复执行 `--install`。
