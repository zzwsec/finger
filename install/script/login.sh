#!/bin/bash

loginListFile="/data/ansible/install/install_list/login_list.txt"
gameListFile="/data/ansible/install/install_list/game_list.txt"
playbookFile="/data/ansible/install/example.yaml"
loginVars="/data/ansible/install/roles/login/vars"
whiteFile="/data/ansible/install/shared_files/etc/white_list.txt"

usage() {
    echo "使用方法：$0 [行号]"
    echo "参数说明："
    echo "  行号           - login_list.txt 中的行号，不写默认为最后一行"
    exit 1
}

error_exit() {
    echo "错误：$1" >&2
    exit "$2"
}

if [[ $# -gt 1 ]]; then
    usage
fi

[[ ! -f "${loginVars}/main.yml.tmp" ]] && error_exit "模板文件不存在" 2

last_line_num=$(sed -n '$=' "$loginListFile" 2>/dev/null)
[[ -z "$last_line_num" ]] && error_exit "login_list.txt文件为空" 3
line_num=${1:-$last_line_num}

if ! [[ "$line_num" =~ ^[0-9]+$ ]] || (( line_num < 1 || line_num > last_line_num )); then
    error_exit "无效行号: $line_num" 4
fi

sed -i '/^$/d' "$loginListFile" || error_exit "清理 login_list.txt 空行失败" 5

get_host_name() {
    local line_num=$1
    awk -v line="$line_num" 'NR==line {print $1}' "$loginListFile" | tr -d '\r\n' \
    || error_exit "无法读取 login_list.txt 第${line_num}行" 6
}

# 通过login_list.txt中的行号获取game_list.txt中对应ID的所有服务编号
to_arr_fun() {
    local line_num=$1
    local modify_id
    modify_id=$(awk -v line_num="${line_num}" 'NR==line_num {print $2}' "$loginListFile" | tr -d '\r\n') \
    || error_exit "无法读取 login_list.txt 第${line_num}行" 7

    if [[ ! $modify_id =~ ^[0-9]+$ ]]; then
        error_exit " 无效的自定义ID: $modify_id" 8
    fi

    local arr=()
    while IFS= read -r line; do
        if [[ $line =~ \[([^]]+)\] ]]; then
            IFS=',' read -ra tmp_arr <<< "${BASH_REMATCH[1]}"
            for item in "${tmp_arr[@]}"; do
                num=$(echo "$item" | xargs)
                [[ ! $num =~ ^[0-9]+$ ]] && error_exit "game_list 中存在非法编号格式: $item" 10
                arr+=("$num")
            done
        fi
    done < <(awk -v mid="$modify_id" '$3 == mid {print $2}' "$gameListFile")

    [[ ${#arr[@]} -eq 0 ]] && error_exit "未找到与modify_id ${modify_id} 对应的game列表" 9
    echo "${arr[@]}"
}

current_ip=$(get_host_name "$line_num")
[[ ! $current_ip =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]] && error_exit "IP地址无效: $current_ip" 7

export current_ip
envsubst < "${loginVars}/main.yml.tmp" > "${loginVars}/main.yml" || error_exit "配置文件生成失败" 8

: > "$whiteFile"
items=($(to_arr_fun "$line_num")) || exit $?
for item in "${items[@]}"; do
    echo "$item" >> "$whiteFile"
done

ansible-playbook -i "${current_ip}," \
    -e "host_name=${current_ip}" \
    -e "role_name=login" \
    "${playbookFile}" || error_exit "Ansible任务失败，任务名：login" 9