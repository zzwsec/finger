# 游戏服务安装工具

基于 Ansible 的游戏服务自动化安装部署工具。通过服务编号快速定位目标主机，自动获取更新包并完成安装部署。

## 目录结构

```
install/
├── game.sh                    # 主安装脚本
├── ansible.cfg                # Ansible 配置文件
├── example.yaml               # Playbook 入口文件
├── README.md                  # 本说明文件
├── install_list/
│   └── game_list.txt           # 主机与服务编号映射表
└── roles/
    ├── game/                  # 游戏服务安装角色
    │   ├── files/
    │   │   └── install.tar.gz  # 游戏安装包
    │   ├── tasks/
    │   │   └── main.yml        # 安装任务
    │   ├── templates/
    │   │   ├── server.app.lua.j2   # 服务配置模板
    │   │   ├── zones.lua.j2        # 区服配置模板
    │   │   └── open_time.lua.j2    # 开服时间模板
    │   └── vars/
    │       └── main.yml.tmp   # 变量配置模板
    └── package/               # 更新包获取角色
        └── tasks/
            └── main.yml        # 打包任务
```

## 前置依赖

- `ansible` — 执行自动化部署
- `envsubst` — 生成配置文件（通常随 `gettext` 包安装）
- `dos2unix` — 处理 `game_list.txt` 的换行符（如从 Windows 编辑）

```bash
# Ubuntu / Debian
sudo apt install ansible gettext dos2unix

# CentOS / RHEL
sudo yum install ansible gettext dos2unix
```

## 使用方法

### 基本用法

```bash
bash game.sh <服务编号> [base|start]
```

### 参数说明

| 参数         | 必填 | 说明                                            |
| ------------ | ---- | ----------------------------------------------- |
| `服务编号`   | 是   | 需在 `game_list.txt` 中存在，例如 `1`           |
| `base`       | 否   | 仅安装不启动（默认）                            |
| `start`      | 否   | 安装并启动                                      |

### 使用示例

```bash
# 安装服务编号 1，不启动
bash game.sh 1

# 安装服务编号 1，并启动
bash game.sh 1 start
```

## 配置说明

### 步骤一：配置 `install_list/game_list.txt`

格式：`IP [服务编号列表] group_id`

```text
192.168.121.101 [1,4,7,10]  1
192.168.121.102 [2,5,8,11]  1
192.168.121.103 [3,6,9,12]  2
```

- **第一列**: 主机内网 IP
- **第二列**: 该主机运行的服务编号列表
- **第三列**: 组编号 `group_id`

> ⚠️ 如果在 Windows 上编辑过此文件，需执行 `dos2unix install_list/game_list.txt` 处理换行符。

### 步骤二：配置 `roles/game/vars/main.yml.tmp`

检查并修改以下关键配置：

| 变量            | 说明                       |
| --------------- | -------------------------- |
| `domain`        | 域名路径                   |
| `thread`        | 线程数                     |
| `pay_notify_url`| kingnet 回调地址           |
| `discovers`     | ZooKeeper 节点列表（ip+端口）|
| `game_db`       | 游戏数据库连接信息         |

以下变量由脚本自动注入，无需手动修改：

- `current_ip` — 目标主机内网 IP
- `game_port` — 游戏端口（基于服务编号偏移计算）
- `server_num` — 服务编号
- `group_id` — 组编号
- `area_id` — 区域编号（等于服务编号）

### 步骤三：检查 `roles/game/templates/`

确认以下模板文件内容正确：

- `server.app.lua.j2` — 服务主配置（IP、端口、数据库、服务发现等）
- `zones.lua.j2` — 区服配置
- `open_time.lua.j2` — 开服时间配置

## 工作流程

1. **校验环境** — 检查依赖命令、配置文件、模板文件是否存在
2. **解析参数** — 根据服务编号查找对应主机 IP、端口、组号
3. **获取更新包** — 从前一个服务编号对应主机打包获取（首个服务跳过）
4. **生成配置** — 通过 `envsubst` 将变量注入 `main.yml.tmp` 生成 `main.yml`
5. **执行安装** — 调用 Ansible 部署游戏服务
6. **清理临时文件** — 脚本退出时自动清理 `main.yml`

## 端口计算规则

游戏端口由服务编号在主机中的位置偏移计算：

```
game_port = 3340 + index × 1000
```

| 主机                  | 服务编号 | index | 端口 |
| --------------------- | -------- | ----- | ---- |
| 192.168.121.101       | 1        | 0     | 3340 |
| 192.168.121.101       | 4        | 1     | 4340 |
| 192.168.121.101       | 7        | 2     | 5340 |

## 远程目录结构

部署完成后，目标主机目录结构如下：

```
/data/
├── update/                           # 更新包临时目录
│   └── install.tar.gz
└── server<编号>/                     # 游戏运行目录
    └── game/
        ├── p8_app_server             # 游戏主程序
        ├── server.sh                 # 启停脚本
        ├── proto/                    # 协议文件
        ├── etc/                      # 配置文件
        │   ├── server.app.lua
        │   ├── server.log.ini
        │   └── zones.lua
        └── lua/                      # Lua 脚本
            └── config/
                └── open_time.lua
```

## 常见问题

### 1. 提示"服务编号 X 未在 game_list.txt 中找到"

检查 `game_list.txt` 中是否包含该编号，确认编号在方括号列表内。

### 2. 提示"模板文件 main.yml.tmp 不存在"

确认 `roles/game/vars/main.yml.tmp` 文件存在且未被删除。

### 3. Ansible 任务失败

检查目标主机 SSH 连接是否正常、`game_list.txt` 中的 IP 是否可达。

### 4. 首个服务编号无需获取更新包

服务编号为列表中的最小值时，脚本会自动跳过获取更新包步骤，使用 `roles/game/files/install.tar.gz` 中已有的安装包。