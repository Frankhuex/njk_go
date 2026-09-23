# 安装PostgreSQL
按需跳过部分步骤
## 1. 安装
Linux:
```shell
sudo apt update
sudo apt install postgresql postgresql-contrib -y
sudo systemctl status postgresql
```
macOS:
本机使用已安装的 PostgreSQL 16（`/Library/PostgreSQL/16`），服务由 `/Library/LaunchDaemons/postgresql-16.plist` 管理。新机器可使用 PostgreSQL 官网提供的 macOS 安装器选择 16；不要重复安装 Homebrew 的其他版本。
```shell
export PATH="/Library/PostgreSQL/16/bin:$PATH" # 同时加入 ~/.zprofile 和 ~/.zshrc
psql --version
```
## 2. 进psql
Linux:
```shell
sudo -i -u postgres #Linux系统切换到postgres用户
psql #进入psql命令行
```
macOS:
```shell
psql -U postgres -d postgres #-U表示数据库用户，-d表示数据库名称
```

## 3. 创建psql用户njk后退出psql
```sql    
create user njk with password '114514';
alter user njk createdb; --允许创建数据库
exit;
```

## 4. 退出Linux用户'postgres'
```shell
exit
```

## 5. 登录psql用户njk
Linux:
```shell
psql -U njk -h localhost -d postgres
```
macOS:
```shell
psql -U njk -h localhost -d postgres
```

## 6. 创建数据库并进入数据库
```sql
create database njk;
\c njk
\dt --显示当前数据库所有表
\d user --查看一个表的列和索引
\q --退出
```

## 7. 添加pgvector扩展
### 安装：
Linux:
```shell
sudo apt update
sudo apt install -y postgresql-server-dev-16 build-essential git
git clone https://github.com/pgvector/pgvector.git
cd pgvector
make PG_CONFIG=/usr/lib/postgresql/16/bin/pg_config
sudo make install PG_CONFIG=/usr/lib/postgresql/16/bin/pg_config
```
macOS:
```shell
git clone --branch v0.8.6 https://github.com/pgvector/pgvector.git
cd pgvector
make PG_CONFIG=/Library/PostgreSQL/16/bin/pg_config OPTFLAGS='' CC=/usr/bin/clang \
  CFLAGS='-O2 -arch x86_64 -arch arm64 -mmacosx-version-min=12' \
  CPPFLAGS="-isysroot $(xcrun --show-sdk-path)" \
  LDFLAGS="-isysroot $(xcrun --show-sdk-path)"
sudo make install PG_CONFIG=/Library/PostgreSQL/16/bin/pg_config
```
### 进psql加扩展：
#### 必须以管理员账号登录psql
Linux:
```shell
sudo -i -u postgres
psql -d njk
```
macOS:
```shell
psql -h localhost -U postgres -d njk # 扩展必须安装在使用它的数据库中
```
#### Linux 和 macOS 的扩展名称均为 vector
```sql
create extension if not exists vector;
```
#### 验证安装：
```sql
SELECT extname FROM pg_extension WHERE extname = 'vector';
```
