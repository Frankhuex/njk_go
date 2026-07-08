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
```shell
brew install postgresql
brew services start postgresql
```
## 2. 切Linux用户，进psql
### 切换到postgres用户
```shell
sudo -i -u postgres
```

### 进入psql命令行
Linux:
```shell
psql
```
macOS:
```shell
psql -d postgres
```

## 3. 创建psql用户njk后退出psql
```sql    
create user njk with password '114514';
alter user njk createdb; --加建表权
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
psql -U njk
```

## 6. 创建数据库并进入数据库
```sql
create database njk;
\c njk;
\q --退出
\dt --显示当前数据库所有表
\d user --查看一个表的列和索引
```

## 7. 添加pgvector扩展
### 安装：
Linux:
```shell
sudo apt update
sudo apt install -y postgresql-server-dev-16 build-essential git
git clone https://github.com/pgvector/pgvector.git
cd pgvector
make
sudo make install
```
macOS:
```shell
brew install pgvector
```
### 进psql加扩展：
#### 必须以管理员账号登录psql
Linux:
```shell
sudo -i -u postgres
```
macOS:
```shell
psql
```
#### 注意Linux中名字为vector，macOS中名字为pgvector
Linux:
```sql
create extension if not exists vector;
```
macOS:
```sql
create extension if not exists pgvector;
```
#### 验证安装：
```sql
SELECT extname FROM pg_extension WHERE extname = 'vector';
```