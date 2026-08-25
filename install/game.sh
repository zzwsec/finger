#!/bin/bash

set -o nounset
umask 077

red='\033[91m'
green='\033[92m'
yellow='\033[93m'
white='\033[0m'

_err_msg() { echo -e "${red}错误${white} $1" >&2; }
_suc_msg() { echo -e "${green}成功${white} $1"; }
_info_msg() { echo -e "${yellow}提示${white} $1"; }

script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd) || exit 1
export ANSIBLE_CONFIG="${script_dir}/ansible.cfg"
game_port_start=3340
gameListFile="${script_dir}/install_list/game_list.txt"
playbookFile="${script_dir}/example.yaml"
gameVars="${script_dir}/roles/game/vars"
gameVarsExample="${gameVars}/main.yml.tmp.example"
declare -A game_ip=()
declare -A game_index=()

cleanup() {
    if [[ -f "${gameVars}/main.yml" ]] && ! rm -f "${gameVars}/main.yml"; then
        _err_msg "临时配置 ${gameVars}/main.yml 清理失败"
    fi
}
trap cleanup EXIT

error_exit() {
    _err_msg "$1"
    exit "${2:-1}"
}

usage() {
    echo "使用方法：bash $0 <服务编号> [base|start]"
    echo "参数说明："
    echo "  服务编号  必填，需要在 game_list.txt 中存在"
    echo "  base      可选，安装并启用 systemd unit，但不立即启动（默认）"
    echo "  start     可选，安装、启用并立即启动 systemd unit"
    exit 1
}

is_valid_ipv4() {
    local ip=$1
    local i

    [[ "$ip" =~ ^(0|[1-9][0-9]{0,2})(\.(0|[1-9][0-9]{0,2})){3}$ ]] || return 1

    for i in ${ip//./ }; do
        (( i <= 255 )) || return 1
    done
}

load_game_list() {
    local line=0
    local count=0
    local ip list extra numbers num index

    while read -r ip list extra; do
        ((line++))

        [[ -z "$ip" || "$ip" == \#* ]] && continue
        [[ -n "$list" && -z "$extra" ]] || error_exit "game_list.txt 第 $line 行应为两列: IP [服务编号列表]" 7

        is_valid_ipv4 "$ip" || error_exit "game_list.txt 第 $line 行 IP 无效: $ip" 7
        [[ "$list" =~ ^\[[1-9][0-9]*(,[1-9][0-9]*)*\]$ ]] || error_exit "game_list.txt 第 $line 行服务编号列表无效: $list" 7

        numbers=${list:1:${#list}-2}

        index=0
        for num in ${numbers//,/ }; do
            [[ -z "${game_ip[$num]+x}" ]] || error_exit "game_list.txt 服务编号重复: $num" 7
            game_ip[$num]=$ip
            game_index[$num]=$index
            ((index++))
        done

        ((count++))
    done < "$gameListFile"

    (( count > 0 )) || error_exit "game_list.txt 没有有效配置" 7
}

run_playbook() {
    local ip=$1
    local role=$2
    local desc=$3
    shift 3
    _info_msg "正在执行: $desc"
    if ansible-playbook -i "${ip}," -e "host_name=${ip}" -e "role_name=${role}" "$@" "${playbookFile}"; then
        _suc_msg "$desc 完成"
    else
        error_exit "$desc 失败" 14
    fi
}

check_env() {
    local command_name

    for command_name in ansible-playbook envsubst grep; do
        command -v "$command_name" &>/dev/null || error_exit "$command_name 未安装" 4
    done
    if [[ ! -f "${gameVars}/main.yml.tmp" ]]; then
        error_exit "生产配置不存在，请先执行: cp ${gameVarsExample} ${gameVars}/main.yml.tmp" 8
    fi
    if grep -q 'CHANGE_ME_' "${gameVars}/main.yml.tmp"; then
        error_exit "main.yml.tmp 仍包含 CHANGE_ME_ 占位符，请填写生产配置" 8
    fi

    [[ -f "$gameListFile" ]] || error_exit "文件 $gameListFile 不存在" 2
    [[ -f "$playbookFile" ]] || error_exit "文件 $playbookFile 不存在" 3
}

[[ $# -eq 0 || $# -gt 2 ]] && usage
server_num=$1
flag=${2:-base}

if [[ "$flag" != "base" && "$flag" != "start" ]]; then
    error_exit "标志位无效: $flag（可选值: base|start）" 6
fi
if [[ ! "$server_num" =~ ^[1-9][0-9]*$ ]]; then
    error_exit "服务编号无效: $server_num（必须为无前导零的正整数）" 6
fi

check_env
load_game_list

current_ip=${game_ip[$server_num]-}
[[ -n "$current_ip" ]] || error_exit "服务编号 $server_num 未在 game_list.txt 中找到" 8
index=${game_index[$server_num]}
game_port=$((game_port_start + index * 1000))
(( game_port <= 65535 )) || error_exit "计算出的端口超出范围: $game_port" 6
pre_server_num=$((server_num - 1))

echo "========================================"
echo "  当前配置"
echo "========================================"
echo "  IP:       $current_ip"
echo "  端口:     $game_port"
echo "  编号:     $server_num"
echo "  模式:     $flag"
echo "========================================"
read -r -p "确认以上配置，按 Enter 继续（Ctrl+C 取消）..." _ || error_exit "未确认配置，任务已取消" 10

if [[ "$pre_server_num" -ge 1 ]]; then
    pre_ip=${game_ip[$pre_server_num]-}
    [[ -n "$pre_ip" ]] || error_exit "前一个服务编号 $pre_server_num 未在 game_list.txt 中找到" 8
    run_playbook "$pre_ip" "package" "从 game$pre_server_num 获取更新包" -e "area_id=$pre_server_num" -e "app_binary=p8_app_server"
else
    _info_msg "首个服务编号 $server_num，跳过远端打包，使用本地 install.tar.gz"
fi

export current_ip game_port server_num
envsubst '$current_ip $game_port $server_num' < "${gameVars}/main.yml.tmp" > "${gameVars}/main.yml" || error_exit "配置文件生成失败" 9

run_playbook "$current_ip" "game" "安装 game$server_num" -t "$flag"
