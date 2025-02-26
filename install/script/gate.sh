#!/bin/bash

listFile="/data/ansible/install/install_list/gate_list.txt"
playbookFile="/data/ansible/install/example.yaml"
gateVars="/data/ansible/install/roles/gate/vars"

usage() {
    echo "使用方法：$0 [行号]"
    echo "参数说明："
    echo "  行号           - gate_list.txt 中的行号，不写默认为最后一行"
    exit 1
}

# 错误处理函数
error_exit() {
    echo "错误：$1" >&2
    exit "$2"
}

# 参数个数检查
if [[ $# -gt 1 ]]; then
    usage
fi

sed -i '/^$/d' "$listFile" || error_exit "清理 gate_list.txt 空行失败" 2

[[ ! -f "${gateVars}/main.yml.tmp" ]] && error_exit "模板文件不存在" 3


# 配置默认行号（最后一行）
last_line_num=$(sed -n '$=' "$listFile" 2>/dev/null)
[[ -z "$last_line_num" ]] && error_exit "gate_list.txt 文件为空" 4
line_num=${1:-$last_line_num}

# 验证行号有效性
if ! [[ "$line_num" =~ ^[0-9]+$ ]] || (( line_num < 1 || line_num > last_line_num )); then
    error_exit "无效行号: $line_num" 5
fi

# 通过行号获取主机名
get_host_name() {
    local line_num=$1
    awk -v line="$line_num" 'NR==line {print $1; exit}' "$listFile" || error_exit "无法读取 gate_list.txt 第${line_num}行" 6
}

# 通过行号获取group_id
get_group_id() {
  local line_num=$1
  awk -v line="$line_num" 'NR==line {print $2; exit}' "$listFile" || error_exit "无法读取 gate_list.txt 第${line_num}行" 7
}

# 有效性验证
verify_args() {
  local current_ip=$1
  local group_id=$2

  if [[ ! "$current_ip" =~ ^[0-9]{1,3}(\.[0-9]{1,3}){3}$ ]]; then
    error_exit "IP地址无效: $current_ip" 8
  fi

  if [[ ! "$group_id" =~ ^[0-9]+$ ]]; then
    error_exit "group_id无效: $group_id" 9
  fi
}

current_ip=$(get_host_name "$line_num")
group_id=$(get_group_id "$line_num")

verify_args "$current_ip" "$group_id"

export current_ip group_id
envsubst < "${gateVars}/main.yml.tmp" > "${gateVars}/main.yml" || error_exit "配置文件生成失败" 10

# 执行Ansible
ansible-playbook -i "${current_ip}," \
    -e "host_name=${current_ip}" \
    -e "role_name=gate" \
    "${playbookFile}" || error_exit "Ansible任务失败，任务名：gate" 11