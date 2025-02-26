#!/bin/bash

game_port_start=3340
gameListFile="/data/ansible/install/install_list/game_list.txt"
gateListFile="/data/ansible/install/install_list/gate_list.txt"
playbookFile="/data/ansible/install/example.yaml"
gameVars="/data/ansible/install/roles/game/vars"

usage() {
    echo "使用方法：$0 [行号]"
    echo "参数说明："
    echo "  行号           - game_list.txt 中的行号，不写默认为最后一行"
    exit 1
}

# 错误处理函数
error_exit() {
    echo "错误：$1" >&2
    exit "$2"
}

# 参数检查
if [[ $# -gt 1 ]]; then
    usage
fi

[[ ! -f "${gameVars}/main.yml.tmp" ]] && error_exit "模板文件不存在" 8

# 配置默认行号（最后一行）
last_line_num=$(sed -n '$=' "$gameListFile" 2>/dev/null)
[[ -z "$last_line_num" ]] && error_exit "game_list.txt 文件为空" 3
line_num=${1:-$last_line_num}

# 验证行号有效性
if ! [[ "$line_num" =~ ^[0-9]+$ ]] || (( line_num < 1 || line_num > last_line_num )); then
    error_exit "无效行号: $line_num" 4
fi

# 删除空行
sed -i '/^$/d' "$gameListFile" || error_exit "清理 game_list.txt 空行失败" 5

# 通过行号获取主机名
get_host_name() {
    local line_num=$1
    awk -v line="$line_num" 'NR==line {print $1; exit}' "$gameListFile" \
    || error_exit "无法读取 game_list.txt 第${line_num}行" 6
}

# game_list.txt中通过行号获取数组
to_arr_fun() {
    local line_num=$1
    local line
    line=$(sed -n "${line_num}p" "$gameListFile") || error_exit "读取 game_list.txt 第${line_num}行失败" 7

    if [[ ! $line =~ \[([^]]+)\] ]]; then
        error_exit "game_list.txt 第${line_num}行格式不正确" 8
    fi

    local arr=()
    IFS=',' read -ra arr <<< "${BASH_REMATCH[1]}"
    [[ ${#arr[@]} -eq 0 ]] && error_exit "第${line_num}行未找到有效编号" 9

    # 验证是否是数字
    local clean_arr=()
    for item in "${arr[@]}"; do
        num=$(echo "$item" | xargs)
        [[ ! $num =~ ^[0-9]+$ ]] && error_exit "非法编号格式: $item" 10
        clean_arr+=("$num")
    done
    echo "${clean_arr[@]}"
}

# 通过行号获取group_id
get_group_id() {
    local line_num=$1
    local group_id
    group_id=$(awk -v line="$line_num" 'NR==line {print $4; exit}' "$gameListFile" \
    || error_exit "无法读取 game_list.txt 第${line_num}行" 6)

    [[ "$group_id" =~ ^[0-9]+$ ]] || error_exit "game_list.txt 第${line_num}行的group_id无效" 6

    # 在 gate_list.txt 文件中检查是否存在对应的group_id
    awk -v id="$group_id" '$2 == id {found=1; exit} END {exit !found}' "$gateListFile" \
    || error_exit "group_id [$group_id] 在 $gateListFile 中不存在" 7

    echo "$group_id"
}

current_ip=$(get_host_name "$line_num")
items=($(to_arr_fun "$line_num")) || exit $?
group_id=$(get_group_id "$line_num")
export group_id

echo "当前正在分发文件，目标ip：${current_ip}"
ansible-playbook -i "${current_ip}," \
    -e "host_name=${current_ip}" \
    -e "role_name=game_base" \
    "${playbookFile}" &>/dev/null \
    && echo "Ansible任务成功，任务名：game_base" \
    || error_exit "Ansible任务失败，任务名：game_base" 14

for index in "${!items[@]}"; do
  {
    server_num="${items[index]}"
    game_port=$((game_port_start + index * 1000))

    echo "当前正在配置：行号=$line_num | IP=$current_ip | 端口=$game_port | 编号=$server_num"

    # 生成独立临时配置文件
    tmp_file="${gameVars}/main.yml.${server_num}"
    export current_ip game_port server_num
    envsubst < "${gameVars}/main.yml.tmp" > "$tmp_file" || error_exit "配置文件生成失败" 9

    ansible-playbook -i "${current_ip}," \
        -e "host_name=${current_ip}" \
        -e "role_name=game" \
        -e "@$tmp_file" \
        "${playbookFile}" &>/dev/null \
        && echo "Ansible任务成功，任务名：game，server_num编号: $server_num" \
        || error_exit "Ansible任务失败，任务名：game，server_num编号: $server_num" 14

    rm -f "$tmp_file"
  }&
done
wait