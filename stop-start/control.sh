#!/bin/bash

# 时间: 2026/7/30

set -o nounset
umask 077

red='\033[91m'
green='\033[92m'
yellow='\033[93m'
white='\033[0m'

_err_msg() { echo -e "\033[41m\033[1m错误${white} $1" >&2; }
_suc_msg() { echo -e "\033[42m\033[1m成功${white} $1"; }
_info_msg() { echo -e "\033[43m\033[1;37m提示${white} $1"; }

err_exit() {
    _err_msg "$1"
    exit "${2:-1}"
}

usage() {
    echo "使用方法：bash $0 <start|stop>"
    echo "  start  按依赖顺序启动全部 P9 服务"
    echo "  stop   按反向依赖顺序停止全部 P9 服务"
    exit 2
}

show_spinner() {
    local message=$1
    local pid=$2
    local spinstr='|/-\'
    local i=0
    local len=${#spinstr}

    while kill -0 "$pid" 2>/dev/null; do
        printf "\r  ${yellow}%s [%s]${white}" "$message" "${spinstr:i++%len:1}"
        sleep 0.1
    done
    printf "\r\033[K"
}

run_playbook() {
    local node_name=$1
    local action=$2
    local playbook_path="playbook/${node_name}/${node_name}-entry.yaml"
    local log_file="runlog/${action}_${node_name}.log"
    local task_pid
    local spinner_pid
    local task_status

    [[ -f "$playbook_path" ]] || err_exit "playbook 文件不存在：$playbook_path" 3

    printf "\n当前时间: %s\n" "$(date '+%F %T')" >> "$log_file"
    ansible-playbook "$playbook_path" --tags "$action" >> "$log_file" 2>&1 &
    task_pid=$!

    show_spinner "正在 ${action} ${node_name}" "$task_pid" &
    spinner_pid=$!

    wait "$task_pid"
    task_status=$?
    wait "$spinner_pid" 2>/dev/null || true

    if [[ "$task_status" -ne 0 ]]; then
        printf "  ${red}%s %s [失败]，详情见 %s${white}\n" "$action" "$node_name" "$log_file"
        exit "$task_status"
    fi

    printf "  ${green}%s %s [完成]${white}\n" "$action" "$node_name"
}

start_all() {
    local node
    # 基础服务先启动，业务入口最后启动。
    for node in zk center game gate login; do
        run_playbook "$node" start
    done
}

stop_all() {
    local node
    # 停止顺序与启动顺序相反。
    for node in login gate game center zk; do
        run_playbook "$node" stop
    done
}

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) ||
    err_exit "无法确定脚本目录"
cd "$script_dir" || err_exit "无法进入脚本目录：$script_dir"
export ANSIBLE_CONFIG="${script_dir}/ansible.cfg"

[[ $# -eq 1 ]] || usage
[[ -d playbook ]] || err_exit "目录不存在：${script_dir}/playbook" 3
[[ -f hosts ]] || err_exit "文件不存在：${script_dir}/hosts" 3
command -v ansible-playbook &>/dev/null || err_exit "ansible-playbook 未安装" 4
mkdir -p runlog || err_exit "无法创建日志目录：${script_dir}/runlog" 5

case "$1" in
    start|stop)
        action=$1
        ;;
    *)
        usage
        ;;
esac

_info_msg "即将 ${action}：zk、center、game、gate、login"
read -r -p "确认执行，按 Enter 继续（Ctrl+C 取消）..." _ ||
    err_exit "操作已取消" 130

if [[ "$action" == "start" ]]; then
    start_all
else
    stop_all
fi

_suc_msg "全部服务 ${action} 完成"
