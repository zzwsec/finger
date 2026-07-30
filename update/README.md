# P9 服务更新工具

该工具使用 Ansible 分发 P9 项目更新文件，并通过 systemd unit 判断服务状态和执行 reload。目前支持以下三种更新方式：

| 更新文件 | 更新范围 | 说明 |
| --- | --- | --- |
| `groups.lua` | game | 更新各 game 实例的 `etc/groups.lua` |
| `increment.tar.gz` | game | 对 game 实例执行增量更新 |
| `alldo.tar.gz` | game、gate、login、zk、center | 对所有服务执行全量更新 |

## 目录说明

```text
update/
├── ansible.cfg       # Ansible 配置
├── hosts             # 主机清单
├── start.sh          # 推荐使用的更新入口
├── file/             # 放置本次更新文件
└── playbook/         # 各服务的 Ansible playbook
```

## 配置主机

主机清单使用“主机别名 + 实际 IP”的形式，因此 Ansible 输出会显示可识别的主机名：

```ini
[game]
gamenode-01 ansible_host=10.46.99.216
gamenode-02 ansible_host=10.46.96.198

[all:vars]
ansible_port=22
```

Ansible 会使用 `ubuntu` 用户连接目标主机，并通过免密 sudo 以 root 身份执行远端任务。修改 IP 或增加主机时，请保持主机别名唯一。

## 准备更新文件

将本次更新文件放入 `file/` 目录。以下三个文件必须且只能存在一个：

```text
groups.lua
increment.tar.gz
alldo.tar.gz
```

### groups.lua 更新

目录内容示例：

```text
file/
└── groups.lua
```

更新后文件会分发到每个 game 实例的：

```text
/data/gameN/etc/groups.lua
```

### 增量或全量更新包

压缩包中的所有内容必须位于 `app/` 目录下：

```text
app/
├── lua_lib/
├── proto/ # 仅全量包
└── ...
```

打包命令：

```bash
tar -czf increment.tar.gz app/
```

全量更新包使用相同结构，文件名改为 `alldo.tar.gz`。

启动脚本会在更新前检查：

- 三种更新文件是否恰好存在一个；
- 压缩包能否正常读取；
- 压缩包是否为空；
- 压缩包内容是否全部位于 `app/` 下；
- 压缩包是否包含 `../` 等不安全路径。

## 执行更新

使用启动脚本：

```bash
bash start.sh
```

脚本会自动识别更新文件，打印更新类型并等待确认。确认后执行对应 playbook。

### 执行顺序

- `groups.lua`：game
- `increment.tar.gz`：game
- `alldo.tar.gz`：game → gate → login → zk → center

每个 playbook 都会先检查目标实例。如果没有发现任何符合规则的实例，任务会直接失败，避免出现“执行成功但没有更新任何服务”的情况。

## 单独执行 playbook

以下命令需要在 `update/` 目录中执行。

更新 game 的 `groups.lua`：

```bash
ansible-playbook playbook/game/game-entry.yaml --tags groups
```

增量更新 game：

```bash
ansible-playbook playbook/game/game-entry.yaml --tags increment
```

全量更新：

```bash
ansible-playbook playbook/game/game-entry.yaml --tags alldo
ansible-playbook playbook/gate/gate-entry.yaml --tags alldo
ansible-playbook playbook/login/login-entry.yaml --tags alldo
ansible-playbook playbook/zk/zk-entry.yaml --tags alldo
ansible-playbook playbook/center/center-entry.yaml --tags alldo
```

## 更新过程

压缩包更新会按以下流程执行：

1. 在目标主机上查找对应的实例目录。
2. 将更新包分发到 `/data/update/`。
3. 删除上一次解压产生的 `/data/update/app`。
4. 将本次更新包解压到 `/data/update/`。
5. 确认与实例同名的 systemd unit 已加载，例如 `game1.service`。
6. 通过 `systemctl is-active` 判断 unit 是否正在运行。
7. 把 `app/` 中的文件复制到每个目标实例。
8. 只对 active 状态的 unit 执行两次 `systemctl reload`，两次操作间隔 1 秒。

## 常见问题

### 提示没有发现实例目录

检查实际目录是否符合当前匹配规则：

| 服务 | 目录规则 |
| --- | --- |
| game | `/data/game[0-9]+` |
| gate | `/data/gate*` |
| login | `/data/login*` |
| zk | `/data/zk/zk[0-9]+` |
| center | `/data/center*` |

### 文件已复制但服务没有 reload

检查：

1. systemd unit 名是否与实例目录名一致，例如 `/data/game1` 对应 `game1.service`。
2. `systemctl is-active game1.service` 是否返回 `active`。
3. unit 的 `ExecStart` 是否指向 P9 程序，例如 `/data/game1/p9_app_server`。
4. `ubuntu` 用户的免密 sudo 是否正常。

### 更新执行到一半失败

`start.sh` 在任意一个 playbook 失败后都会立即退出。修复问题后，可单独执行失败服务的 playbook，也可以重新运行 `start.sh`。
