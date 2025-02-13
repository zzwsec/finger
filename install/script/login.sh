#!/bin/bash

login_port=3346
listFile="/data/ansible/install/list.txt"
loginVars="/data/ansible/install/roles/login/vars"
whiteFile="/data/ansible/install/shared_files/etc/white_list.txt"
playbookFile="/data/ansible/install/example.yaml"

usage() {
    echo -e "使用方法：$0 [line_number]\n"
    echo "参数说明："
    echo "  line_number  - 服务器在list.txt中的行号，不写默认为最后一行"
    exit 1
}

# 错误处理函数
error_exit() {
    echo "错误：$1" >&2
    exit "$2"
}

# 参数检查（允许0或1个参数）
if [[ $# -gt 1 ]]; then
    usage
fi

# 配置默认行号（最后一行）
last_line_num=$(sed -n '$=' "$listFile" 2>/dev/null)
[[ -z "$last_line_num" ]] && error_exit "list.txt文件为空" 3
line_num=${1:-$last_line_num}

# 验证行号有效性
if ! [[ "$line_num" =~ ^[0-9]+$ ]] || (( line_num < 1 || line_num > last_line_num )); then
    error_exit "无效行号: $line_num" 10
fi

# 删除空行
sed -i '/^$/d' "$listFile" || error_exit "清理list.txt空白行失败" 11

# 通过行号获取主机名
get_host_name() {
    local line_num=$1
    awk -v line="$line_num" 'NR==line {print $1; exit}' "$listFile" || error_exit "无法读取list.txt第${line_num}行" 4
}

# 通过行号获取数组
to_arr_fun() {
    local line_num=$1
    local line
    line=$(sed -n "${line_num}p" "$listFile") || error_exit "读取list.txt第${line_num}行失败" 5

    if [[ ! $line =~ \[([^]]+)\] ]]; then
        error_exit "list.txt第${line_num}行格式不正确" 6
    fi

    local arr=()
    IFS=',' read -ra arr <<< "${BASH_REMATCH[1]}"
    [[ ${#arr[@]} -eq 0 ]] && error_exit "第${line_num}行未找到有效编号" 7

    # 清理空格并验证数字
    local clean_arr=()
    for item in "${arr[@]}"; do
        num=$(echo "$item" | xargs)
        [[ ! $num =~ ^[0-9]+$ ]] && error_exit "非法编号格式: $item" 12
        clean_arr+=("$num")
    done
    echo "${clean_arr[@]}"
}

main() {
    current_ip=$(get_host_name "$line_num")
    [[ ! -f "${loginVars}/main.yml.tmp" ]] && error_exit "模板文件不存在" 8

    # 获取并处理数组
    items=($(to_arr_fun "$line_num")) || exit $?

    export current_ip login_port
    envsubst < "${loginVars}/main.yml.tmp" > "${loginVars}/main.yml" || error_exit "配置文件生成失败" 9

    : > "$whiteFile"
    
    for item in "${items[@]}"; do
        echo "$item" >> "$whiteFile"
    done

    # 执行Ansible
    ansible-playbook -i "${current_ip}," \
        -e "host_name=${current_ip}" \
        -e "role_name=login" \
        "${playbookFile}" || error_exit "Ansible任务失败，任务名：login" 14
}

main
