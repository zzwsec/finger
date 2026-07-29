#!/bin/bash

# 时间: 2026/7/30

set -o nounset
umask 077

# 颜色定义
red='\033[91m'
green='\033[92m'
yellow='\033[93m'
white='\033[0m'

_err_msg() { echo -e "\033[41m\033[1m错误${white} $1" >&2; }
_suc_msg() { echo -e "\033[42m\033[1m成功${white} $1"; }
_info_msg() { echo -e "\033[43m\033[1;37m提示${white} $1"; }

# 路径定义
script_dir=$(dirname "$(realpath "${BASH_SOURCE[0]}")")
export ANSIBLE_CONFIG="${script_dir}/ansible.cfg"
game_port_start=3349
gameListFile="${script_dir}/install_list/game_list.txt"
playbookFile="${script_dir}/example.yaml"
gameVars="${script_dir}/roles/game/vars"
gameVarsExample="${gameVars}/main.yml.tmp.example"

# 清理临时配置文件
cleanup() {
    if [[ -f "${gameVars}/main.yml" ]] && ! rm -f "${gameVars}/main.yml"; then
        _err_msg "临时配置 ${gameVars}/main.yml 清理失败"
    fi
}

trap cleanup EXIT

# 错误处理
error_exit() {
    _err_msg "$1"
    exit "${2:-1}"
}

# 使用说明
usage() {
    echo "使用方法：bash $0 <服务编号> [base|start]"
    echo "参数说明："
    echo "  服务编号  必填，需要在 game_list.txt 中存在"
    echo "  base      可选，仅安装不启动（默认）"
    echo "  start     可选，安装并启动"
    exit 1
}

# 通过服务编号获取主机 IP
get_ip() {
    local target=$1
    local ip
    ip=$(awk -v target="$target" '
        NF > 0 {
            ip = $1
            line = $0
            sub(/.*\[/, "", line)
            sub(/\].*/, "", line)
            n = split(line, arr, /[, ]+/)
            for (i = 1; i <= n; i++) {
                if (arr[i] != "" && arr[i] == target) {
                    print ip
                    exit 0
                }
            }
        }
    ' "$gameListFile")
    if [[ -z "$ip" ]]; then
        _err_msg "服务编号 $target 未在 game_list.txt 中找到"
        return 8
    fi
    printf '%s\n' "$ip"
}

# 获取服务编号在主机中的偏移量（用于端口计算）
get_index() {
    local target_ip=$1
    local target_num=$2
    local index
    index=$(awk -v ip="$target_ip" -v num="$target_num" '
        NF > 0 && $1 == ip {
            line = $0
            sub(/.*\[/, "", line)
            sub(/\].*/, "", line)
            n = split(line, arr, /[, ]+/)
            for (i = 1; i <= n; i++) {
                if (arr[i] != "" && arr[i] == num) {
                    print i - 1
                    exit 0
                }
            }
        }
    ' "$gameListFile")
    if [[ -z "$index" ]]; then
        _err_msg "服务编号 $target_num 在 IP $target_ip 上无效"
        return 8
    fi
    printf '%s\n' "$index"
}

# 简单校验安装列表格式
validate_game_list() {
    awk '
        { sub(/\r$/, "") }
        NF == 0 { next }
        {
            valid_rows++
            if (NF != 2 ||
                $1 !~ /^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$/ ||
                $2 !~ /^\[[1-9][0-9]*(,[1-9][0-9]*)*\]$/) {
                invalid = 1
            }
        }
        END { exit !(valid_rows > 0 && !invalid) }
    ' "$gameListFile" ||
        error_exit "game_list.txt 格式无效，应为: IP [服务编号列表]" 7
}

# 运行 Ansible Playbook
run_playbook() {
    local ip=$1
    local role=$2
    local desc=$3
    shift 3
    _info_msg "正在执行: $desc"
    if ansible-playbook -i "${ip}," -e "host_name=${ip}" -e "role_name=${role}" "$@" "${playbookFile}"; then
        _suc_msg "$desc 完成"
    else
        error_exit "$desc 失败" 14
    fi
}

# 环境检查
check_env() {
    local config_mode
    local command_name

    for command_name in ansible-playbook envsubst awk grep stat; do
        command -v "$command_name" &>/dev/null || error_exit "$command_name 未安装" 4
    done

    if [[ ! -f "${gameVars}/main.yml.tmp" ]]; then
        error_exit "生产配置不存在，请先执行: cp ${gameVarsExample} ${gameVars}/main.yml.tmp" 8
    fi
    if grep -q "CHANGE_ME_" "${gameVars}/main.yml.tmp"; then
        error_exit "main.yml.tmp 仍包含 CHANGE_ME_ 占位符，请填写生产配置" 8
    fi
    config_mode=$(stat -c '%a' "${gameVars}/main.yml.tmp") || error_exit "无法检查 main.yml.tmp 权限" 8
    [[ "$config_mode" == "600" ]] || error_exit "main.yml.tmp 权限必须为 600，请执行 chmod 600 ${gameVars}/main.yml.tmp" 8
    [[ ! -f "$gameListFile" ]] && error_exit "文件 $gameListFile 不存在" 2
    [[ ! -f "$playbookFile" ]] && error_exit "文件 $playbookFile 不存在" 3
}

# ========== 主流程 ==========

# 参数校验
[[ $# -eq 0 || $# -gt 2 ]] && usage
server_num=$1
flag=${2:-base}

if [[ "$flag" != "base" && "$flag" != "start" ]]; then
    error_exit "标志位无效: $flag（可选值: base|start）" 6
fi
if [[ ! "$server_num" =~ ^[1-9][0-9]*$ ]]; then
    error_exit "服务编号无效: $server_num（必须为无前导零的正整数）" 6
fi

check_env
validate_game_list

# 获取配置信息
current_ip=$(get_ip "$server_num") || exit $?
index=$(get_index "$current_ip" "$server_num") || exit $?
game_port=$((game_port_start + index * 1000))
(( game_port <= 65535 )) || error_exit "计算出的端口超出范围: $game_port" 6

# 获取前一个服务编号（用于获取更新包）
pre_server_num=$((server_num - 1))

# 显示配置确认信息
echo "========================================"
echo "  当前配置"
echo "========================================"
echo "  IP:     $current_ip"
echo "  端口:   $game_port"
echo "  编号:   $server_num"
echo "  启动:   $flag"
echo "========================================"
read -r -p "确认以上配置，按 Enter 继续..." _ ||
    error_exit "未确认配置，任务已取消" 10

# 获取更新包
if [[ "$pre_server_num" -ge 1 ]]; then
    pre_ip=$(get_ip "$pre_server_num") || exit $?
    run_playbook "$pre_ip" "package" "从 game$pre_server_num 获取更新包" -e "area_id=$pre_server_num"
else
    _info_msg "首个服务编号 $server_num，跳过获取更新包，使用已有更新包"
fi

# 生成配置文件
_info_msg "正在生成配置文件"
export current_ip game_port server_num
envsubst '$current_ip $game_port $server_num' \
    < "${gameVars}/main.yml.tmp" > "${gameVars}/main.yml" ||
    error_exit "配置文件生成失败" 9

# 执行安装
run_playbook "$current_ip" "game" "安装 game$server_num" -t "$flag"

_suc_msg "game$server_num 安装完成"
