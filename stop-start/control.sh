#!/usr/bin/env bash

set -o nounset

red='\033[91m'
green='\033[92m'
yellow='\033[93m'
white='\033[0m'

_err_msg() { echo -e "${red}错误 $1${white}" >&2; }
_info_msg() { echo -e "${yellow}提示 $1${white}"; }

script_path=$(readlink -f -- "${BASH_SOURCE[0]}") || exit 1
script_dir=$(cd -- "$(dirname -- "$script_path")" && pwd) || exit 1
export ANSIBLE_CONFIG="${script_dir}/ansible.cfg"
playbook_file="${script_dir}/playbook/service.yaml"
inventory_file="${script_dir}/hosts"
runlog_dir="${script_dir}/runlog"

declare -A unit_pattern=(
    [login]='login*.service'
    [gate]='gate*.service'
    [game]='game*.service'
    [cross]='crossserver*.service'
    [gm]='gmserver*.service'
    [global]='global*.service'
    [log]='logserver*.service'
    [zk]='zk*.service'
    [api]='apiserver*.service'
)

declare -A save_on_stop=(
    [game]=1
    [cross]=1
    [gm]=1
    [global]=1
    [log]=1
)

cleanup() {
    local status=$1
    (( status == 0 )) && rm -rf -- "$runlog_dir"
}
trap 'cleanup $?' EXIT

err_exit() {
    _err_msg "$1"
    exit "${2:-1}"
}

_show_spinner() {
    local spinstr='|/-\'
    local msg=$1
    local pid=$2
    local i=0
    local len=${#spinstr}

    while kill -0 "$pid" 2>/dev/null; do
        printf "\r${yellow}[%s] %s [%s]${white}" \
            "$(date '+%T')" "$msg" "${spinstr:i++%len:1}"
        sleep 0.1
    done

    printf "\r\033[K"
}

main() {
    local option=$1
    local node_name=${2:-}
    local target=${node_name:-全部服务}
    local start_time
    local end_time

    _info_msg "执行 $option $target，按 Enter 继续..."
    read -r || err_exit "未收到确认，已取消操作" 2
    mkdir -p "$runlog_dir" || err_exit "日志目录 $runlog_dir 创建失败" 1

    start_time=$(date +%s)
    printf "开始时间: %s\n\n" "$(date '+%F %T')"

    if [[ -n "$node_name" ]]; then
        update_option "$node_name" "$option"
    elif [[ "$option" == start ]]; then
        update_start
    else
        update_stop
    fi

    end_time=$(date +%s)
    printf "\n结束时间: %s\n" "$(date '+%F %T')"
    printf "总耗时: %d 秒\n" "$((end_time - start_time))"
}

update_start() {
    local node_name
    for node_name in zk log global gm cross game gate login api; do
        update_option "$node_name" start
    done
}

update_stop() {
    local node_name
    for node_name in login gate game cross gm global log zk api; do
        update_option "$node_name" stop
    done

    for node_name in login gate game cross gm global log zk api; do
        update_option "$node_name" clean
    done

    update_option all journal
}

update_option() {
    local node_name=$1
    local flag=$2
    local log_file="${runlog_dir}/${flag}_${node_name}.log"
    local batch_size='100%'
    local pattern="${unit_pattern[$node_name]-}"

    if [[ "$flag" == stop && -n "${save_on_stop[$node_name]-}" ]]; then
        batch_size=2
    fi

    printf "开始时间: %s\n" "$(date '+%F %T')" >> "$log_file"

    ansible-playbook -i "$inventory_file" "$playbook_file" \
        -e "service_group=$node_name" \
        -e "unit_pattern=$pattern" \
        -e "batch_size=$batch_size" \
        -t "$flag" >> "$log_file" 2>&1 &
    local task_pid=$!

    _show_spinner "正在：${flag} --> ${node_name} node" "$task_pid" &
    local spinner_pid=$!

    wait "$task_pid"
    local task_status=$?

    kill "$spinner_pid" 2>/dev/null
    wait "$spinner_pid" 2>/dev/null || true
    printf "\r\033[K"

    if (( task_status != 0 )); then
        printf "${red}[%s] ${flag} --> %s node [失败]，执行过程见 %s${white}\n" \
            "$(date '+%T')" "$node_name" "$log_file"
        exit 1
    fi

    printf "${green}[%s] ${flag} --> %s node [完成]${white}\n" \
        "$(date '+%T')" "$node_name"
}

[[ $# -ge 1 && $# -le 2 ]] || err_exit "参数数量错误" 2

case $1 in
    start|stop)
        ;;
    *)
        err_exit "操作类型错误: $1（可选值: start|stop）" 2
        ;;
esac

if [[ $# -eq 2 ]]; then
    [[ -n "${unit_pattern[$2]+x}" ]] ||
        err_exit "服务类型错误: $2" 2
fi

command -v ansible-playbook &>/dev/null || err_exit "ansible-playbook 未安装" 1
[[ -f "$ANSIBLE_CONFIG" ]] || err_exit "文件 $ANSIBLE_CONFIG 不存在" 1
[[ -f "$inventory_file" ]] || err_exit "文件 $inventory_file 不存在" 1
[[ -f "$playbook_file" ]] || err_exit "playbook 文件 $playbook_file 不存在" 1

main "$1" "${2:-}"
