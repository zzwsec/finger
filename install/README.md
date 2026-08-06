# P9 游戏服务安装工具

部署结构

```text
/data/game<编号>/
├── p9_app_server
├── etc/
│   ├── server.app.lua
│   ├── server.log.ini
│   ├── server.lua
│   ├── services.lua
│   └── zones.lua
├── log/
├── lua_lib/
│   └── config/profile.lua
└── proto/

/etc/systemd/system/game<编号>.service
```

## 使用

安装控制机需要 `ansible-playbook` 和 `envsubst`。
目标服务器不需要开放 root 登录，但必须存在 `ubuntu` 用户，并允许该用户免密执行 `sudo`：

```bash
ssh ubuntu@10.20.30.8 'sudo -n true'
```

该命令必须无交互成功，然后才能执行安装：

```bash
# 仅安装并启用服务，不启动
bash game.sh <服务编号>

# 安装、启用并启动服务
bash game.sh <服务编号> start
```

脚本会读取 `install_list/game_list.txt`。每行格式为：

```text
10.20.30.8 [1,4,7]
```

两部分依次为主机内网 IP 和该主机上的服务编号列表。
端口以 `3349` 为起点，同一主机上的后续实例每个增加 `1000`；
所有 game 的 `group_id` 和 `game_index_num` 均固定为 `1`。

## 安装包来源

安装非首个服务时，脚本默认从前一个服务 `/data/gameN` 获取以下内容：

- `p9_app_server`
- `proto/`
- `etc/`
- `lua_lib/`

打包结果保存到 `roles/game/files/install.tar.gz`。
首个服务没有前置实例，必须先手工将一个非空的 P9 安装包放到该位置；空包会在部署前被拒绝。
运行期的 `log/` 和数据库状态文件不会复制到新实例。

## 配置

仓库不保存数据库密码。首次克隆后，从无敏感信息的示例创建
仅存在于生产机的私有配置：

```bash
cp roles/game/vars/main.yml.tmp.example roles/game/vars/main.yml.tmp
chmod 600 roles/game/vars/main.yml.tmp
vim roles/game/vars/main.yml.tmp
```

替换以下占位符：

- `CHANGE_ME_DISCOVERY_IP`：服务发现地址
- `CHANGE_ME_DB_HOST`：数据库地址
- `CHANGE_ME_DB_PASSWORD`：数据库密码

填写时保留示例中的 YAML 单引号；如果值本身包含单引号，需要写成两个
连续的单引号。

脚本自动注入目标 IP、端口、服务编号、组编号和实例编号。Ansible 会生成：

- `etc/server.app.lua`
- `etc/zones.lua`
- `lua_lib/config/profile.lua`（只替换 `open_server_time`）
- `/etc/systemd/system/gameN.service`

只安装全新实例，不会停止或覆盖已有的 `/data/gameN`。
如果目标目录已存在，部署会直接失败。
`base` 只安装并启用 systemd 服务但不启动进程；
`start` 会安装、启动服务并等待 `systemctl is-active` 返回 `active`。

如果安装过程在解压后失败，会保留未完成的 `/data/gameN`；
确认没有需要保留的内容后，手工删除该目录再重试。

## 常见检查

```bash
systemctl status game1.service --no-pager -l
journalctl -u game1.service -o cat
```

若提示安装包为空，先确认存在可用的前置实例，或手工放置完整安装包。
若找不到服务编号，检查 `install_list/game_list.txt`，并确保文件使用 Unix 换行符。