#!/bin/bash

# time: 2025/3/11

set -o nounset

red='\033[91m'
green='\033[92m'
yellow='\033[93m'
white='\033[0m'

_err_msg() { echo -e "\033[41m\033[1m警告${white} $*"; }
_suc_msg() { echo -e "\033[42m\033[1m成功${white} $*"; }
_info_msg() { echo -e "\033[43m\033[1;37m提示${white} $*"; }

err_exit() {
    _err_msg "$1"
    exit "$2"
}

_show_spinner() {
    local spinstr='⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏'
    local msg="$1"
    local pid="$2"
    local i=0
    local len=${#spinstr}
    while kill -0 "$pid" 2>/dev/null; do
        printf "\r  ${yellow}%s [%s]${white}" "$msg" "${spinstr:i++%len:1}"
        sleep 0.1
    done
    printf "\r\033[K"
}

print_info_and_execute_playbook() {
    local option="$1"
    if [ "$option" = "group" ]; then
        _info_msg "检测到 groups.lua 执行更新 group.lua 操作, 按任意键继续..."
        read -r
        update_group_lua
    elif [ "$option" = "increment" ]; then
        _info_msg "检测到 increment.tar.gz 执行更新操作, 按任意键继续..."
        read -r
        update_increment
    else
        err_exit "异常值" 3
    fi
}

update_option() {
    local file_name="$1"
    local node_name="$2"
    local playbook_path="$3"
    local tag="$4"

    [[ ! -f "$playbook_path" ]] && err_exit "playbook 文件 $playbook_path 不存在" 1
    local log_file="./runlog/update_${node_name}.log"
    printf "当前时间: %s\n" "$(date +%F\ %T)" >> "$log_file"
    if [ -n "$tag" ]; then
        ansible-playbook "$playbook_path" -t "$tag" >> "$log_file" 2>&1 &
    else
        ansible-playbook "$playbook_path" >> "$log_file" 2>&1 &
    fi
    local task_pid=$!

    if ! kill -0 "$task_pid" 2>/dev/null; then
        err_exit "无法启动 Ansible" 1
    fi

    _show_spinner "正在更新 ${file_name} for ${node_name} node" "$task_pid" &
    local spinner_pid=$!
    wait "$task_pid"
    local task_status=$?

    # 停止并清理动画
    kill "$spinner_pid" 2>/dev/null
    wait "$spinner_pid" 2>/dev/null || true
    printf "\r\033[K" # 清理动画行

    if [ "$task_status" -ne 0 ]; then
        printf "  ${red}更新 %s for %s node [失败], 执行过程见 %s${white}\n" "$file_name" "$node_name" "$log_file"
        # _err_msg "更新 ${node_name} 节点的 ${file_name} 失败（退出码: $task_status）"
        exit 1
    else
        printf "  ${green}更新 %s for %s node [完成]${white}\n" "$file_name" "$node_name"
        # _suc_msg "成功更新 ${node_name} 节点的 ${file_name}"
    fi
}

update_group_lua() {
    update_option "groups.lua" "cross" "playbook/cross/cross-entry.yaml" "groups"
    update_option "groups.lua" "game" "playbook/game/game-entry.yaml" "groups"
}

update_increment() {
    update_option "increment.tar.gz" "cross" "playbook/cross/cross-entry.yaml" "increment"
    update_option "increment.tar.gz" "game" "playbook/game/game-entry.yaml" "increment"
    update_option "increment.tar.gz" "gm" "playbook/gm/gm-entry.yaml" ""
    update_option "increment.tar.gz" "log" "playbook/log/log-entry.yaml" ""
}

[[ ! -d ./file/ ]] && err_exit "错误：目录 ./file/ 不存在" 1

command -v ansible &>/dev/null || err_exit "错误：ansible 未安装" 1

[[ ! -d ./runlog/ ]]; mkdir -p ./runlog

group_stat=$(find ./file/ -name "groups.lua" -type f | wc -l)
increment_stat=$(find ./file/ -name "increment.tar.gz" -type f | wc -l)

if [[ "$group_stat" -eq 1 && "$increment_stat" -eq 0 ]]; then
    print_info_and_execute_playbook "group"
elif [[ "$group_stat" -eq 0 && "$increment_stat" -eq 1 ]]; then
    tar tf ./file/increment.tar.gz | sed -n '1p' |grep -q "app/" || err_exit "increment.tar.gz 未包含 app 目录" 2
    print_info_and_execute_playbook "increment"
elif [[ "$group_stat" -eq 1 && "$increment_stat" -eq 1 ]]; then
    err_exit "groups.lua 和 increment.tar.gz 同时存在, 请删除或移动其中一个" 2
else
    err_exit "groups.lua 或 increment.tar.gz 不存在, 请检查 file 目录" 2
fi

rm -rf ./runlog/* && rm -rf ./file/*