#!/bin/bash

# 时间: 2025/3/11
err_exit() {
    echo "$1" >&2
    exit "$2"
}

print_info_and_execute_playbook() {
    local option="$1"
    if [ "$option" = "group" ]; then
        echo "检测到 groups.lua 执行更新 group.lua 操作，按任意键继续..."
        read -r || true  # 防止无输入时出错
        update_group_lua
    elif [ "$option" = "increment" ]; then
        echo "检测到 increment.tar.gz 执行更新操作，按任意键继续..."
        read -r || true
        update_increment
    else
        err_exit "异常值: $option" 3  # 更详细的错误信息
    fi
}

update_option() {
    local file_name="$1"
    local node_name="$2"
    local playbook_path="$3"
    local tag="$4"

    [[ ! -f "$playbook_path" ]] && err_exit "playbook 文件 $playbook_path 不存在" 1
    if [ -n "$tag" ]; then
        ansible-playbook "$playbook_path" -t "$tag" || err_exit "Ansible 执行失败: $playbook_path" 4
    else
        ansible-playbook "$playbook_path" || err_exit "Ansible 执行失败: $playbook_path" 4
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

# 检查 ./file/ 目录是否存在
[[ ! -d ./file/ ]] && err_exit "错误：目录 ./file/ 不存在" 1

# 检查 ansible 是否安装
command -v ansible &>/dev/null || err_exit "错误：ansible 未安装" 1

# 统计文件数量
group_stat=$(find ./file/ -name "groups.lua" -type f | wc -l)
increment_stat=$(find ./file/ -name "increment.tar.gz" -type f | wc -l)

# 根据文件存在情况执行相应操作
if [[ "$group_stat" -eq 1 && "$increment_stat" -eq 0 ]]; then
    print_info_and_execute_playbook "group"
elif [[ "$group_stat" -eq 0 && "$increment_stat" -eq 1 ]]; then
    tar tf ./file/increment.tar.gz | sed -n '1p' | grep -q "app/" || err_exit "increment.tar.gz 未包含 app 目录" 2
    print_info_and_execute_playbook "increment"
elif [[ "$group_stat" -eq 1 && "$increment_stat" -eq 1 ]]; then
    err_exit "groups.lua 和 increment.tar.gz 同时存在，请删除或移动其中一个" 2
else
    err_exit "groups.lua 或 increment.tar.gz 不存在，请检查 file 目录" 2
fi
