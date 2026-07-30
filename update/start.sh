#!/bin/bash

# 时间: 2026/7/30
err_exit() {
    echo "$1" >&2
    exit "$2"
}

validate_archive() {
    local archive_path="$1"
    local archive_entries
    local entry
    local normalized_entry

    archive_entries=$(tar -tzf "$archive_path") || err_exit "无法读取压缩包: $archive_path" 2
    [[ -z "$archive_entries" ]] && err_exit "压缩包为空: $archive_path" 2

    while IFS= read -r entry; do
        normalized_entry="${entry#./}"
        case "$normalized_entry" in
            app|app/*) ;;
            *) err_exit "$archive_path 包含 app 目录之外的内容: $entry" 2 ;;
        esac

        case "/$normalized_entry/" in
            *"/../"*) err_exit "$archive_path 包含不安全路径: $entry" 2 ;;
        esac
    done <<< "$archive_entries"
}

print_info_and_execute_playbook() {
    local option="$1"
    if [ "$option" == "group" ]; then
        echo "检测到 groups.lua 执行更新 group.lua 操作，按任意键继续..."
        read -r || true
        update_group_lua
    elif [ "$option" == "increment" ]; then
        echo "检测到 increment.tar.gz 执行更新操作，按任意键继续..."
        read -r || true
        update_increment
    elif [ "$option" == "all" ]; then
        echo "检测到 alldo.tar.gz 执行更新操作，按任意键继续..."
        read -r || true
        update_all
    else
        err_exit "异常值: $option" 3
    fi
}

update_option() {
    local node_name="$1"
    local playbook_path="$2"
    local tag="$3"

    [[ ! -f "$playbook_path" ]] && err_exit "playbook 文件 $playbook_path 不存在" 1
    ansible-playbook "$playbook_path" -t "$tag" || err_exit "Ansible 执行失败 playbook路径为: $playbook_path, 节点名: $node_name" 4

}

update_group_lua() {
    update_option "game" "playbook/game/game-entry.yaml" "groups"
}

update_all() {
    update_option "game" "playbook/game/game-entry.yaml" "alldo"
    update_option "gate" "playbook/gate/gate-entry.yaml" "alldo"
    update_option "login" "playbook/login/login-entry.yaml" "alldo"
    update_option "zk" "playbook/zk/zk-entry.yaml" "alldo"
    update_option "center" "playbook/center/center-entry.yaml" "alldo"
}

update_increment() {
    update_option "game" "playbook/game/game-entry.yaml" "increment"
}


# ========== 主流程 ==========

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) || err_exit "无法确定脚本目录" 1
cd "$script_dir" || err_exit "无法进入脚本目录: $script_dir" 1

# 检查 ./file/ 目录是否存在
[[ ! -d ./file/ ]] && err_exit "错误：目录 ./file/ 不存在" 1

# 检查 ansible-playbook 是否安装
command -v ansible-playbook &>/dev/null || err_exit "错误：ansible-playbook 未安装" 1

# 三种更新文件必须且只能存在一个
update_file_count=0
[[ -f ./file/groups.lua ]] && ((update_file_count += 1))
[[ -f ./file/increment.tar.gz ]] && ((update_file_count += 1))
[[ -f ./file/alldo.tar.gz ]] && ((update_file_count += 1))

if [[ "$update_file_count" -ne 1 ]]; then
    err_exit "groups.lua、increment.tar.gz、alldo.tar.gz 必须且只能存在一个，请检查 file 目录" 2
fi

# 根据文件存在情况执行相应操作
if [[ -f ./file/groups.lua ]]; then
    print_info_and_execute_playbook "group"
elif [[ -f ./file/increment.tar.gz ]]; then
    validate_archive ./file/increment.tar.gz
    print_info_and_execute_playbook "increment"
elif [[ -f ./file/alldo.tar.gz ]]; then
    validate_archive ./file/alldo.tar.gz
    print_info_and_execute_playbook "all"
fi
