#!/bin/bash

# 时间: 2025/3/27
err_exit() {
    echo "$1" >&2
    exit "$2"
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
    update_option "cross" "playbook/cross/cross-entry.yaml" "groups"
    update_option "game" "playbook/game/game-entry.yaml" "groups"
}

update_all() {
    update_option "cross" "playbook/cross/cross-entry.yaml" "alldo"
    update_option "game" "playbook/game/game-entry.yaml" "alldo"
    update_option "gm" "playbook/gm/gm-entry.yaml" "alldo"
    update_option "log" "playbook/log/log-entry.yaml" "alldo"
    update_option "gate" "playbook/gate/gate-entry.yaml" "alldo"
    update_option "login" "playbook/login/login-entry.yaml" "alldo"
    update_option "zk" "playbook/zk/zk-entry.yaml" "alldo"
    update_option "global" "playbook/global/global-entry.yaml" "alldo"
}

update_increment() {
    update_option "cross" "playbook/cross/cross-entry.yaml" "increment"
    update_option "game" "playbook/game/game-entry.yaml" "increment"
    update_option "gm" "playbook/gm/gm-entry.yaml" "increment"
    update_option "log" "playbook/log/log-entry.yaml" "increment"
}


# 检查 ./file/ 目录是否存在
[[ ! -d ./file/ ]] && err_exit "错误：目录 ./file/ 不存在" 1

# 检查 ansible 是否安装
command -v ansible &>/dev/null || err_exit "错误：ansible 未安装" 1

# 统计文件数量
group_stat=$(find ./file/ -name "groups.lua" -type f | wc -l)
increment_stat=$(find ./file/ -name "increment.tar.gz" -type f | wc -l)
all_stat=$(find ./file/ -name "alldo.tar.gz" -type f | wc -l)

# 根据文件存在情况执行相应操作
if [[ "$group_stat" -eq 1 && "$increment_stat" -eq 0 && "$all_stat" -eq 0 ]]; then
    print_info_and_execute_playbook "group"
elif [[ "$group_stat" -eq 0 && "$increment_stat" -eq 1 && "$all_stat" -eq 0 ]]; then
    tar tf ./file/increment.tar.gz | sed -n '1p' | grep -q "app/" || err_exit "increment.tar.gz 未包含 app 目录" 2
    print_info_and_execute_playbook "increment"
elif [[ "$group_stat" -eq 0 && "$increment_stat" -eq 0 && "$all_stat" -eq 1 ]]; then
    tar tf ./file/alldo.tar.gz | sed -n '1p' | grep -q "app/" || err_exit "alldo.tar.gz 未包含 app 目录" 2
    print_info_and_execute_playbook "all"
elif [[ "$group_stat" -eq 1 && "$increment_stat" -eq 1  && "$all_stat" -eq 1 ]]; then
    err_exit "groups.lua 和 increment.tar.gz 和 alldo.tar.gz 同时存在，请删除或移动其中一个" 2
else
    err_exit "groups.lua 或 increment.tar.gz 或 alldo.tar.gz 不存在，请检查 file 目录" 2
fi
