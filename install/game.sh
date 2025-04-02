#!/bin/bash

script_dir=$(dirname "$(realpath "${BASH_SOURCE[0]}")")
game_port_start=3340
gameListFile="${script_dir}/install_list/game_list.txt"
playbookFile="${script_dir}/example.yaml"
gameVars="${script_dir}/roles/game/vars"

usage() {
    echo "使用方法：bash $0 [服务编号]"
    echo "参数说明："
    echo "  1.要安装的服务编号 -- 需要在 game_list.txt 文件中存在"
    echo "  2.是否在安装后启动 -- 默认不启动"
    exit 1
}

# 错误处理函数
error_exit() {
    echo "错误：$1" >&2
    exit "${2:-1}"
}

# 通过服务编号获取主机ip
get_ip() {
    local server_num=$1
    local current_ip
    while read -r line;do
      current_ip=$(awk '{print $1}' <<< "$line")
      if awk -F '[][]' '{print $2}' <<<"$line" | tr ',' ' ' | grep -q -w "$server_num" ;then
        echo "$current_ip"
        return 0
      fi
    done < "$gameListFile"
    error_exit "输入的服务编号无效" 8
}

# 通过服务编号获取 group_id
get_group_id() {
    local group_id
    group_id=$(awk -v current_ip="$current_ip" '$1==current_ip {print $NF; exit}' "$gameListFile")
    [[ -z "$group_id" ]] && error_exit "未找到匹配的 IP 或 group_id" 6
    [[ "$group_id" =~ ^[0-9]+$ ]] || error_exit "game_list.txt 中的group_id无效" 6
    echo "$group_id"
}

#获取服务编号的偏移量
get_index() {
  local line
  IFS=' ' read -ra line <<< "$(awk -v current_ip="$current_ip" '$1==current_ip {print $2; exit}' "$gameListFile" | tr -d '[]' | tr ',' ' ')"
  for i in "${!line[@]}";do
    if [ "$server_num" -eq "${line[$i]}" ];then
      echo "$i"
      return 0
    fi
  done
  error_exit "服务编号无效" 8
}

check_env() {
  [[ ! -f "${gameVars}/main.yml.tmp" ]] && error_exit "模板文件不存在" 8
  [[ ! -f "$gameListFile" ]] && error_exit "$gameListFile 不存在" 2
  [[ ! -f "$playbookFile" ]] && error_exit "$playbookFile 不存在" 3
  sed -i '/^$/d' "$gameListFile" || error_exit "清理 game_list.txt 空行失败" 5
}

check_env

[[ $# -gt 2 ]] && usage
flag=${2:-base}
server_num=$1
if [[ "$flag" != "base" ]] && [[ "$flag" != "start" ]]; then
    error_exit "输入标志位无效" 6
fi
if [[ ! $server_num =~ ^[0-9]+$ ]]; then
    error_exit "输入服务编号无效" 6
fi

current_ip=$(get_ip "$server_num")
group_id=$(get_group_id)
index=$(get_index)
game_port=$((game_port_start + index * 1000))

pre_server_num=$((server_num-1))
pre_ip=$(get_ip "$pre_server_num")

read -r -p "当前配置：IP=$current_ip | 端口=$game_port | 编号=$server_num | 是否启动：$flag |输入任意值继续任务"

echo "正在从 game$pre_server_num 获取更新包" && sleep 1
if ! ansible-playbook -i "${pre_ip}," -e "host_name=${pre_ip}" -e "role_name=package" -e "area_id=$pre_server_num" "${playbookFile}"; then
    error_exit "Ansible任务失败，任务名：package，server_num编号: $pre_server_num" 14
else
    echo "Ansible任务成功，任务名：package，server_num编号: $pre_server_num"
fi

clear
echo "成功获取更新包，正在安装" && sleep 1
export current_ip game_port server_num group_id
envsubst < "${gameVars}/main.yml.tmp" > "${gameVars}/main.yml" || error_exit "配置文件生成失败" 9

if ! ansible-playbook -i "${current_ip}," -e "host_name=${current_ip}" -e "role_name=game" "${playbookFile}" -t "$flag"; then
    error_exit "Ansible任务失败，任务名：game，server_num编号: $server_num" 14
else
    echo "Ansible任务成功，任务名：game，server_num编号: $server_num"
fi

rm -f "${gameVars}/main.yml"