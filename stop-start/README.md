# P9 服务启停工具

> **实验性说明**
>
> 当前版本根据最新版 `install/` 和 `update/` 改造，尚未在生产环境完成整套
> 启停验证。正式使用前请核对 `hosts`、实例目录、systemd unit 名称及服务
> 顺序；建议先单独执行各服务 playbook，并检查 `runlog/` 和服务状态。

该工具与当前 `install/`、`update/` 的部署结构保持一致，通过 systemd
启停 `game`、`gate`、`login`、`zk` 和 `center` 服务。

## 环境要求

- 控制机已安装 `ansible-playbook`。
- 目标机存在 `ubuntu` 用户，并允许该用户免密执行 `sudo`。
- 各实例已安装同名 systemd unit，例如 `/data/game1` 对应
  `game1.service`。
- `hosts` 中的主机分组和地址已按实际环境配置。

可先检查 SSH 和 sudo：

```bash
ssh ubuntu@10.46.99.216 'sudo -n true'
```

## 使用

在任意目录均可调用脚本：

```bash
bash /path/to/stop-start/control.sh stop
bash /path/to/stop-start/control.sh start
```

脚本会显示操作范围并等待确认。每个服务类型按顺序执行；任一步失败后立即
退出，详细输出保存在 `runlog/<操作>_<服务>.log`。

启动顺序：

```text
zk → center → game → gate → login
```

停止顺序：

```text
login → gate → game → center → zk
```

## 单独操作

以下命令需要在 `stop-start/` 目录执行：

```bash
ansible-playbook playbook/game/game-entry.yaml --tags stop
ansible-playbook playbook/game/game-entry.yaml --tags start
```

将 `game` 替换为 `gate`、`login`、`zk` 或 `center`，即可单独操作对应服务。

## 实例识别规则

| 服务 | 实例目录 | systemd unit |
| --- | --- | --- |
| game | `/data/game[0-9]+` | `gameN.service` |
| gate | `/data/gate*` | 与目录同名 |
| login | `/data/login*` | 与目录同名 |
| zk | `/data/zk/zk[0-9]+` | `zkN.service` |
| center | `/data/center*` | 与目录同名 |

每个 playbook 都会先确认至少发现一个实例，并检查对应 systemd unit 已加载。
启动后会等待 unit 进入 `active`，停止后会等待 unit 离开 `active`。
