## 更新类型

| 更新文件 | 更新范围 |
| --- | --- |
| `groups.lua` | cross、game、api |
| `increment.tar.gz` | cross、game、gm、log、api |
| `alldo.tar.gz` | cross、game、gm、log、api、gate、login、zk、global |

`file/` 中以上三个文件必须且只能存在一个。压缩包中的内容必须全部位于顶层 `app/` 目录中，例如：

```text
app/
├── etc/
├── lua/
└── ...
```

示例打包命令：

```bash
tar zcf increment.tar.gz app
```

## 执行更新

`hosts` 先在 `[all]` 中按照“服务名 + 编号”的格式定义别名和 IP，再将别名加入对应服务组，例如：

```ini
[all]
login_01 ansible_host=192.168.121.102
game_01 ansible_host=192.168.121.102

[game]
game_01

[login]
login_01
```

### 更新全部适用服务

不传服务参数时，脚本根据 `file/` 中的工件更新该模式下的全部适用服务：

```bash
bash start.sh
```

执行顺序如下：

| 工件 | 执行顺序 |
| --- | --- |
| `groups.lua` | cross → game → api |
| `increment.tar.gz` | cross → game → gm → log → api |
| `alldo.tar.gz` | cross → game → gm → log → api → gate → login → zk → global |

### 单独更新一个服务

将服务类型作为唯一参数传入：

```bash
bash start.sh game
bash start.sh cross
```

完整用法：

```text
bash start.sh [cross|game|gm|log|api|gate|login|zk|global]
```

服务必须受到当前工件支持，否则脚本会在连接远端前退出：

```text
groups.lua        → cross、game、api
increment.tar.gz → cross、game、gm、log、api
alldo.tar.gz      → cross、game、gm、log、api、gate、login、zk、global
```

脚本可以使用用绝对路径调用：

```bash
bash /绝对路径/update/start.sh game
```

也可以把脚本软链接到系统 PATH：

```bash
ln -s /绝对路径/update/start.sh /usr/local/bin/an-update
an-update game
```

脚本会解析软链接的真实路径，配置、inventory、playbook 和 `file/` 仍从原始 `update/` 目录读取。

脚本会校验更新文件并等待确认。任一服务更新失败后立即停止；有失败日志时保留 `runlog/`，全部成功后自动删除该目录。

## systemd unit 规则

| 服务 | unit 匹配规则 |
| --- | --- |
| login | `login*.service` |
| gate | `gate*.service` |
| game | `game*.service` |
| cross | `crossserver*.service` |
| gm | `gmserver*.service` |
| global | `global*.service` |
| log | `logserver*.service` |
| zk | `zk*.service` |
| api | `apiserver*.service` |

每类服务的更新过程如下：

1. 使用 `systemctl list-unit-files` 查找 unit；某台主机未找到匹配 unit 时跳过该主机并继续。
2. 使用 `systemctl show` 读取每个 unit 的 `WorkingDirectory`，并要求目录位于 `/data/` 下。
3. 更新 `groups.lua`，或由控制节点把压缩包直接传输并解压到该工作目录；解压时去掉顶层 `app/`。
4. 仅对更新前处于 active 状态的 unit 执行 `systemctl reload`；unit 自身的 `ExecReload` 决定具体 reload 信号和次数。

原本未运行的 unit 只更新文件，不会被启动。

例如安装工具创建的 `/data/game1` 会由 `game1.service` 的 `WorkingDirectory` 自动定位。
