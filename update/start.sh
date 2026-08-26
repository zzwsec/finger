#!/usr/bin/env bash

set -o nounset
set -o pipefail

red='\033[91m'
green='\033[92m'
yellow='\033[93m'
white='\033[0m'

_err_msg() { echo -e "${red}错误 $1${white}" >&2; }
_info_msg() { echo -e "${yellow}提示 $1${white}"; }

script_path=$(readlink -f -- "${BASH_SOURCE[0]}") || exit 1
script_dir=$(cd -- "$(dirname -- "$script_path")" && pwd) || exit 1
playbook_file="${script_dir}/playbook/service.yaml"
inventory_file="${script_dir}/hosts"
file_dir="${script_dir}/file"
runlog_dir="${script_dir}/runlog"
export ANSIBLE_CONFIG="${script_dir}/ansible.cfg"

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

declare -A mode_services=(
    [groups]='cross game api'
    [increment]='cross game gm log api'
    [alldo]='cross game gm log api gate login zk global'
)

err_exit() {
    _err_msg "$1"
    exit "${2:-1}"
}

cleanup() {
    (( $1 == 0 )) && rm -rf -- "$runlog_dir"
}
trap 'cleanup $?' EXIT

validate_archive() {
    local archive_path=$1
    local archive_entries
    local entry
    local normalized_entry

    archive_entries=$(tar tf "$archive_path") || err_exit "无法读取压缩包: $archive_path" 2
    [[ -n "$archive_entries" ]] || err_exit "压缩包为空: $archive_path" 2

    while IFS= read -r entry; do
        normalized_entry=${entry#./}
        case "$normalized_entry" in
            app|app/*) ;;
            *) err_exit "$archive_path 包含 app 目录之外的内容: $entry" 2 ;;
        esac
    done <<< "$archive_entries"
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

update_option() {
    local node_name=$1
    local tag=$2
    local log_file="${runlog_dir}/${tag}_${node_name}.log"

    printf "开始时间: %s\n" "$(date '+%F %T')" >> "$log_file"

    ansible-playbook -i "$inventory_file" "$playbook_file" \
        -e "service_group=$node_name" \
        -e "unit_pattern=${unit_pattern[$node_name]}" \
        -t "$tag" >> "$log_file" 2>&1 &
    local task_pid=$!

    _show_spinner "正在：${tag} --> ${node_name} node" "$task_pid" &
    local spinner_pid=$!

    wait "$task_pid"
    local task_status=$?

    kill "$spinner_pid" 2>/dev/null
    wait "$spinner_pid" 2>/dev/null || true
    printf "\r\033[K"

    if (( task_status != 0 )); then
        printf "${red}[%s] ${tag} --> %s node [失败]，执行过程见 %s${white}\n" \
            "$(date '+%T')" "$node_name" "$log_file"
        exit 1
    fi

    printf "${green}[%s] ${tag} --> %s node [完成]${white}\n" \
        "$(date '+%T')" "$node_name"
}

command -v ansible-playbook &>/dev/null || err_exit "ansible-playbook 未安装" 1
[[ $# -le 1 ]] || err_exit "参数数量错误，用法: bash start.sh [服务类型]" 2

requested_service=${1:-}
if [[ -n "$requested_service" ]]; then
    case "$requested_service" in
        login|gate|game|cross|gm|global|log|zk|api) ;;
        *) err_exit "服务类型错误: $requested_service" 2 ;;
    esac
fi

[[ -f "$ANSIBLE_CONFIG" ]] || err_exit "文件 $ANSIBLE_CONFIG 不存在" 1
[[ -f "$inventory_file" ]] || err_exit "文件 $inventory_file 不存在" 1
[[ -f "$playbook_file" ]] || err_exit "playbook 文件 $playbook_file 不存在" 1
[[ -d "$file_dir" ]] || err_exit "目录 $file_dir 不存在" 1

update_file_count=0
[[ -f "$file_dir/groups.lua" ]] && ((update_file_count += 1))
[[ -f "$file_dir/increment.tar.gz" ]] && ((update_file_count += 1))
[[ -f "$file_dir/alldo.tar.gz" ]] && ((update_file_count += 1))
(( update_file_count == 1 )) || err_exit "groups.lua、increment.tar.gz、alldo.tar.gz 必须且只能存在一个" 2

if [[ -f "$file_dir/groups.lua" ]]; then
    mode=groups
elif [[ -f "$file_dir/increment.tar.gz" ]]; then
    mode=increment
    validate_archive "$file_dir/increment.tar.gz"
else
    mode=alldo
    validate_archive "$file_dir/alldo.tar.gz"
fi

if [[ -n "$requested_service" ]]; then
    case " ${mode_services[$mode]} " in
        *" $requested_service "*) ;;
        *) err_exit "$mode 更新不支持 $requested_service 服务" 2 ;;
    esac
fi

target=${requested_service:-全部适用服务}
_info_msg "检测到 ${mode} 更新，目标: ${target}，按 Enter 继续..."
read -r || err_exit "未收到确认，已取消更新" 2

mkdir -p "$runlog_dir" || err_exit "日志目录 $runlog_dir 创建失败" 1

start_time=$(date +%s)
printf "开始时间: %s\n\n" "$(date '+%F %T')"

if [[ -n "$requested_service" ]]; then
    update_option "$requested_service" "$mode"
else
    for service in ${mode_services[$mode]}; do
        update_option "$service" "$mode"
    done
fi

end_time=$(date +%s)

printf "\n结束时间: %s\n" "$(date '+%F %T')"
printf "总耗时: %d 秒\n" "$((end_time - start_time))"
