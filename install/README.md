## 环境要求
目标主机要求：

- 允许控制机通过 SSH 免密登录 root
- 安装 Python 3

## 首次配置

### 1. 配置实例列表

编辑 `install_list/game_list.txt`：

```text
192.168.121.101 [1,4,7,10]
192.168.121.102 [2,5,8,11]
192.168.121.103 [3,6,9,12]
```

每行依次为：

1. 目标主机 IP；
2. 部署在该主机上的服务编号。

服务编号必须是正整数且不能重复。端口按照编号在该主机列表中的位置计算：

```text
game_port = 3340 + 索引 × 1000
```

### 2. 创建变量文件

示例文件不包含密码：

```bash
cd install
cp roles/game/vars/main.yml.tmp.example roles/game/vars/main.yml.tmp
```

修改 `main.yml.tmp` 中所有 `CHANGE_ME_` 配置。该文件已被 `.gitignore`
排除，不应提交到 Git。

脚本运行时只替换以下动态变量：

- `${current_ip}`
- `${game_port}`
- `${server_num}`

## 执行安装

仅安装并启用开机启动，不立即启动：

```bash
bash game.sh 1
# 或
bash game.sh 1 base
```

安装、启用并立即启动：

```bash
bash game.sh 1 start
```

为了避免覆盖实例，目标目录或同名 unit 已存在时安装会直接失败，不会自动删除或覆盖。