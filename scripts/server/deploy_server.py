#!/usr/bin/env python3
"""Linux release controller: Python standard library and user systemd."""
import argparse
import fcntl
import json
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import time
import urllib.error
import urllib.request

SERVICE = 'njk.service'
VERSION = re.compile(r'(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\Z')


def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def atomic_text(path, text):
    tmp = path.with_name(path.name + '.tmp')
    tmp.write_text(text)
    os.replace(tmp, path)


def version_tuple(value):
    if not VERSION.fullmatch(value):
        raise RuntimeError('版本格式应为 2.3.3')
    return tuple(map(int, value.split('.')))


def read_env(path):
    # 与 Go 应用相同的普通 KEY=VALUE 规则；不执行 shell。
    values = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if line and not line.startswith('#') and '=' in line:
            key, value = line.split('=', 1)
            values[key.strip()] = value.strip().strip('\"\x27')
    return values


class Deployment:
    def __init__(self, root):
        self.root = root.resolve()
        self.releases = self.root / 'releases'
        self.settings = json.loads((self.root / 'server.json').read_text())
        self.values = read_env(self.root / '.env')

    def log(self, message):
        line = time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()) + ' ' + message
        print(line, flush=True)
        with (self.root / 'deploy-history.log').open('a') as f:
            f.write(line + '\n')

    def systemctl(self, *args, capture=False):
        result = run(['systemctl', '--user', *args], text=True,
                     stdout=subprocess.PIPE if capture else None)
        return result.stdout.strip() if capture else None

    def release(self, version):
        version_tuple(version)
        path = self.releases / version
        if not path.is_dir() or path.is_symlink():
            raise RuntimeError('版本目录不存在或为软链接: ' + version)
        return path

    def current(self):
        link = self.root / 'current'
        if not link.is_symlink() or link.resolve().parent != self.releases:
            raise RuntimeError('current 未指向 releases 下的版本')
        return link.resolve().name

    def switch(self, version):
        link = self.root / 'current.next'
        link.unlink(missing_ok=True)
        link.symlink_to('releases/' + version)
        os.replace(link, self.root / 'current')

    def legacy_pids(self):
        expected = Path(self.settings['legacy_binary']).resolve()
        found = []
        for proc in Path('/proc').iterdir():
            if not proc.name.isdigit():
                continue
            try:
                if proc.stat().st_uid == os.getuid() and (proc / 'exe').resolve() == expected:
                    found.append(int(proc.name))
            except (OSError, RuntimeError):
                pass
        return found

    def stop_legacy(self):
        for pid in self.legacy_pids():
            self.log(f'停止旧手动进程 pid={pid}')
            os.kill(pid, signal.SIGTERM)
        deadline = time.monotonic() + 30
        while self.legacy_pids() and time.monotonic() < deadline:
            time.sleep(.2)
        for pid in self.legacy_pids():
            os.kill(pid, signal.SIGKILL)
        deadline = time.monotonic() + 5
        while self.legacy_pids() and time.monotonic() < deadline:
            time.sleep(.2)
        if self.legacy_pids():
            raise RuntimeError('旧进程未退出')

    def health(self, version, timeout=30):
        expected = (self.releases / version / 'njk_go').resolve()
        port = int((self.values.get('WS_ADDR') or '11003').rsplit(':', 1)[-1])
        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
        deadline = time.monotonic() + timeout
        consecutive = 0
        while time.monotonic() < deadline:
            try:
                pid = int(self.systemctl('show', SERVICE, '-p', 'MainPID', '--value', capture=True))
                if pid <= 0 or Path(f'/proc/{pid}/exe').resolve() != expected:
                    raise RuntimeError('服务进程版本尚未就绪')
                # 普通 HTTP 返回 426，验证应用现有 WebSocket 入口。
                try:
                    response = opener.open(f'http://127.0.0.1:{port}/', timeout=2)
                    response.close()
                    raise RuntimeError('WebSocket 入口返回意外状态码')
                except urllib.error.HTTPError as exc:
                    if exc.code != 426:
                        raise
                consecutive += 1
                if consecutive >= 3:
                    return
            except (OSError, ValueError, RuntimeError, subprocess.SubprocessError):
                consecutive = 0
            time.sleep(1)
        raise RuntimeError('健康检查超时：进程版本或 WebSocket 入口未就绪')

    def deploy(self, version, rollback=False, check=False):
        state = self.root / 'deployment-state.json'
        if state.exists() and json.loads(state.read_text()).get('phase') == 'switching':
            raise RuntimeError('上次发布被中断，请先执行 bash restart.sh --recover')
        release = self.release(version)
        old = self.current()
        self.release(old)
        highest_file = self.root / 'highest-version'
        highest = highest_file.read_text().strip()
        if not rollback and version_tuple(version) <= version_tuple(highest):
            raise RuntimeError('发布版本必须高于历史成功版本 ' + highest + '；恢复旧版本请使用 --rollback')
        if rollback and not (release / '.deployed').exists():
            raise RuntimeError('回退目标尚未成功运行过')
        self.log(f'预检 {old} -> {version}')
        if check:
            return
        atomic_text(state, json.dumps({'old': old, 'target': version, 'phase': 'switching'}) + '\n')
        try:
            self.stop_legacy()
            self.switch(version)
            self.systemctl('restart', SERVICE)
            self.health(version)
        except BaseException:
            self.log('启动失败，恢复程序 ' + old)
            self.switch(old)
            self.systemctl('reset-failed', SERVICE)
            self.systemctl('restart', SERVICE)
            self.health(old)
            atomic_text(state, json.dumps({'old': old, 'target': version, 'phase': 'rolled_back'}) + '\n')
            self.log('程序已恢复 ' + old)
            raise
        (release / '.deployed').touch()
        if not rollback:
            atomic_text(highest_file, version + '\n')
        atomic_text(state, json.dumps({'old': old, 'target': version, 'phase': 'success'}) + '\n')
        self.log('发布成功 ' + version)

    def recover(self):
        state = self.root / 'deployment-state.json'
        data = json.loads(state.read_text())
        if data.get('phase') != 'switching':
            raise RuntimeError('没有需要恢复的中断发布')
        old = data['old']
        self.release(old)
        self.stop_legacy()
        self.switch(old)
        self.systemctl('reset-failed', SERVICE)
        self.systemctl('restart', SERVICE)
        self.health(old)
        data['phase'] = 'recovered'
        atomic_text(state, json.dumps(data) + '\n')
        self.log('已恢复程序 ' + old)


def install(root, legacy, baseline):
    version_tuple(baseline)
    if not re.fullmatch(r'/[A-Za-z0-9_/-]+', str(root)):
        raise RuntimeError('安装路径仅支持普通绝对路径')
    for command in ('systemctl', 'loginctl', 'rsync', 'flock'):
        if not shutil.which(command):
            raise RuntimeError('缺少命令: ' + command)
    if not (root / '.env').is_file():
        raise RuntimeError('缺少服务器 .env')
    settings_file = root / 'server.json'
    if settings_file.exists():
        raise RuntimeError('已安装；更新控制脚本无需再次执行 --install')
    unit_dir = Path.home() / '.config/systemd/user'
    unit = unit_dir / SERVICE
    if unit.exists():
        raise RuntimeError('systemd 服务已存在，请先核对: ' + str(unit))
    binary = legacy / ('njk_go-linux-amd64-v' + baseline)
    if not binary.is_file():
        raise RuntimeError('找不到旧版本程序: ' + str(binary))
    release = root / 'releases' / baseline
    release.mkdir(parents=True, exist_ok=False)
    shutil.copy2(binary, release / 'njk_go')
    (release / '.deployed').touch()
    for name in ('images', 'runtime', 'temp'):
        target = root / name
        if target.exists() or target.is_symlink():
            continue
        if (legacy / name).is_dir():
            target.symlink_to(legacy / name, target_is_directory=True)
        else:
            target.mkdir()
    (root / 'current').symlink_to('releases/' + baseline)
    atomic_text(root / 'highest-version', baseline + '\n')
    if not (root / 'versions.log').exists():
        atomic_text(root / 'versions.log', baseline + '\n')
    root.joinpath('.env').chmod(0o600)
    atomic_text(settings_file, json.dumps({'legacy_binary': str(binary)}, indent=2) + '\n')
    unit_dir.mkdir(parents=True, exist_ok=True)
    unit.write_text(f'''[Unit]
Description=NJK Go bot
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
WorkingDirectory={root}
ExecStart={root}/current/njk_go
Restart=on-failure
RestartSec=3
TimeoutStopSec=30
KillMode=control-group
UMask=0077
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=default.target
''')
    run(['loginctl', 'enable-linger', os.environ['USER']])
    run(['systemctl', '--user', 'daemon-reload'])
    run(['systemctl', '--user', 'enable', SERVICE])
    print('安装完成，保留旧进程继续运行；下次发布自动接替', flush=True)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('version', nargs='?')
    parser.add_argument('--check', action='store_true')
    parser.add_argument('--rollback', action='store_true')
    parser.add_argument('--recover', action='store_true')
    parser.add_argument('--install', action='store_true')
    parser.add_argument('--legacy-dir', type=Path, default=Path('/home/frank/njk_go'))
    parser.add_argument('--baseline', default='2.3.2')
    args = parser.parse_args()
    if not args.install and not args.recover and args.version is None:
        parser.error('请指定版本号')
    with (args.root / 'deploy.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        if args.install:
            install(args.root.resolve(), args.legacy_dir.resolve(), args.baseline)
        elif args.recover:
            Deployment(args.root).recover()
        else:
            Deployment(args.root).deploy(args.version, args.rollback, args.check)


if __name__ == '__main__':
    try:
        main()
    except (Exception, KeyboardInterrupt) as exc:
        print('[ERROR] ' + str(exc), file=sys.stderr)
        sys.exit(1)
