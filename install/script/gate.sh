#!/bin/bash

gate_port=3344
listFile="/data/ansible/install/list.txt"
gateVars="/data/ansible/install/roles/gate/vars"
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

main() {
    current_ip=$(get_host_name "$line_num")
    [[ ! -f "${gateVars}/main.yml.tmp" ]] && error_exit "模板文件不存在" 8

    export current_ip gate_port
    envsubst < "${gateVars}/main.yml.tmp" > "${gateVars}/main.yml" || error_exit "配置文件生成失败" 9

    # 执行Ansible
    ansible-playbook -i "${current_ip}," \
        -e "host_name=${current_ip}" \
        -e "role_name=gate" \
        "${playbookFile}" || error_exit "Ansible任务失败，任务名：gate" 14
}

main
