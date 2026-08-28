`open` 监控当前 game 的注册人数和付费人数，达到任一阈值后自动开放下一个 game。Go 只负责业务编排，远程主机变更全部由 Ansible playbook 完成。

## 目录结构

```text
open/
├── cmd/open/                 程序入口
├── internal/
│   ├── app/                  开服流程和重试策略
│   ├── automation/           go-ansible 调用封装
│   ├── cdn/                  CDN 客户端
│   ├── config/               环境变量加载与校验
│   ├── metrics/              MySQL 指标查询
│   ├── state/                当前 game 状态文件
│   └── topology/             game 拓扑解析
├── automation/
│   ├── ansible.cfg
│   └── playbooks/            playbook 和 Jinja2 模板
├── config/
│   └── games.txt             game 和 IP 映射
├── state/
│   └── current_game          当前已经开放的 game
├── Dockerfile
└── docker-compose.yml
```

## 运行约定

game 使用：

```text
/data/gameN
gameN.service
```

login 使用 `login*.service`。启动、停止、reload 和工作目录查询全部通过 systemd 完成。

白名单和创建限制文件分别位于 login unit 的 `WorkingDirectory` 下的 `etc/white_list.txt` 和 `etc/limit_create.txt`，不配置固定绝对路径。

安装前会检查目标 `/data/gameN` 和 `gameN.service` 均不存在；任一目标已经存在都会停止安装，避免覆盖现有实例。

## 开服流程

达到注册或付费阈值后依次执行：

1. 从当前 game 打包程序文件。
2. 将安装包直接解压到下一个 game 节点，写入开服时间，并安装、启用、启动 `gameN.service`。
3. 从 login 节点的白名单中删除新 game，并 reload 正在运行的 `login*.service`。
4. 刷新新 game 的 CDN。
5. 等待 `OPEN_LIMIT_DELAY`。
6. 将上一个 game 写入 login 节点的创建限制文件，再次 reload login。
7. 所有步骤成功后，原子更新 `state/current_game`。

每个外部步骤最多执行四次，前三次失败后分别等待 5 秒、10 秒和 15 秒。进程收到 `SIGINT` 或 `SIGTERM` 后会取消正在执行的数据库、HTTP、等待或 Ansible 操作。

开服开始后，程序会在 `state/current_game.pending` 中原子记录下一步操作。打包产物保存在持久化挂载的 `state/game.tar.gz`，因此容器重启后安装步骤仍能读取。某一步连续失败、进程退出或容器重启后，会从失败步骤继续，不会重新执行已经完成的步骤。全部步骤完成并更新 `state/current_game` 后，pending 文件自动删除；下一次开服打包时会覆盖旧安装包。

例如 game63 已安装但 CDN 刷新失败时，pending 状态为：

```json
{"current_game":62,"next_game":63,"next_step":"cdn"}
```

下一轮会直接重试 CDN，成功后继续等待、更新创建限制并提交 game63 状态。

## 拓扑文件

`config/games.txt` 每行格式：

```text
HOST [GAME_ID,GAME_ID]
```

示例：

```text
10.46.99.216 [1,4]
10.46.96.198 [2,5]
```

文件支持空行和以 `#` 开始的行尾注释；重复 game ID 会在启动时被拒绝。login IP 和统一的 group ID 分别通过 `OPEN_LOGIN_HOST`、`OPEN_GAME_GROUP_ID` 配置。

## 环境变量

公共配置：

| 变量 | 含义 | 默认值 |
| --- | --- | --- |
| `OPEN_CDN_URL` | CDN 刷新地址 | 必填 |
| `OPEN_LOGIN_HOST` | 唯一的 login 节点 IP | 必填 |
| `OPEN_REGISTER_THRESHOLD` | 注册人数阈值 | `2000` |
| `OPEN_RECHARGE_THRESHOLD` | 付费人数阈值 | `100` |
| `OPEN_MONEY_THRESHOLD` | 单个玩家累计付费金额阈值 | `6` |
| `OPEN_POLL_INTERVAL` | 数据库检查间隔，使用 Go duration | `10s` |
| `OPEN_LIMIT_DELAY` | 白名单开放到创建限制更新的间隔 | `300s` |
| `OPEN_GAMES_FILE` | game 拓扑文件 | `/open/config/games.txt` |
| `OPEN_STATE_FILE` | 当前 game 状态文件 | `/open/state/current_game` |
| `OPEN_AUTOMATION_DIR` | Ansible 目录 | `/open/automation` |

日志数据库配置：

```text
OPEN_LOG_DATABASE_HOST
OPEN_LOG_DATABASE_PORT (默认 3306)
OPEN_LOG_DATABASE_USER (默认 root)
OPEN_LOG_DATABASE_PASSWORD
OPEN_LOG_DATABASE_NAME (默认 cbt4_log)
```

自动安装配置：

```text
OPEN_DOMAIN (默认 /p8)
OPEN_GAME_THREADS (默认 8)
OPEN_PAY_NOTIFY_URL
OPEN_ZK_ENDPOINTS
OPEN_GAME_DATABASE_HOST
OPEN_GAME_DATABASE_PORT (默认 3306)
OPEN_GAME_DATABASE_USER (默认 root)
OPEN_GAME_DATABASE_PASSWORD
OPEN_GAME_DATABASE_NAME_PREFIX (默认 cbt4_game_)
OPEN_GAME_INDEX_COUNT (默认 2)
OPEN_GAME_BASE_PORT (默认 3340)
OPEN_GAME_GROUP_ID (默认 1)
```

`OPEN_ZK_ENDPOINTS` 使用逗号分隔的 `host:port`：

```text
10.0.0.1:2881,10.0.0.2:2882,10.0.0.3:2883
