#!/bin/bash

listFile="/data/ansible/install/list.txt"
playbookFile="/data/ansible/install/example.yaml"

# game使用的
game_port_start=3340
gameVars="/data/ansible/install/roles/game/vars"

# gate使用的
gate_port=3344
gateVars="/data/ansible/install/roles/gate/vars"

# login使用的
login_port=3346
loginVars="/data/ansible/install/roles/login/vars"
whiteFile="/data/ansible/install/shared_files/etc/white_list.txt"

usage() {
    echo "使用方法：$0 [安装的服务类型] [行号]"
    echo "参数说明："
    echo "  安装的服务类型  - 可选范围（all, login, gate, game）"
    echo "  行号           - 服务器在list.txt中的行号，不写默认为最后一行"
    exit 1
}

# 错误处理函数
error_exit() {
    echo "错误：$1" >&2
    exit "$2"
}

# 参数检查（允许0或1或2个参数）
if [[ $# -gt 2 ]]; then
    usage
fi

# 配置默认安装类型（all）
if [[ $# -eq 0 ]]; then
    install_type="all"
else
    install_type="$1"
fi

# 验证安装类型有效性
if ! [[ "$install_type" =~ ^(all|login|gate|game)$ ]]; then
    usage
fi

# 配置默认行号（最后一行）
last_line_num=$(sed -n '$=' "$listFile" 2>/dev/null)
[[ -z "$last_line_num" ]] && error_exit "list.txt文件为空" 3
line_num=${2:-$last_line_num}

# 验证行号有效性
if ! [[ "$line_num" =~ ^[0-9]+$ ]] || (( line_num < 1 || line_num > last_line_num )); then
    error_exit "无效行号: $line_num" 4
fi

# 删除空行
sed -i '/^$/d' "$listFile" || error_exit "清理list.txt空行失败" 5

# 通过行号获取主机名
get_host_name() {
    local line_num=$1
    awk -v line="$line_num" 'NR==line {print $1; exit}' "$listFile" || error_exit "无法读取list.txt第${line_num}行" 6
}

# 通过行号获取数组
to_arr_fun() {
    local line_num=$1
    local line
    line=$(sed -n "${line_num}p" "$listFile") || error_exit "读取list.txt第${line_num}行失败" 7

    if [[ ! $line =~ \[([^]]+)\] ]]; then
        error_exit "list.txt第${line_num}行格式不正确" 8
    fi

    local arr=()
    IFS=',' read -ra arr <<< "${BASH_REMATCH[1]}"
    [[ ${#arr[@]} -eq 0 ]] && error_exit "第${line_num}行未找到有效编号" 9

    # 清理空格并验证数字
    local clean_arr=()
    for item in "${arr[@]}"; do
        num=$(echo "$item" | xargs)
        [[ ! $num =~ ^[0-9]+$ ]] && error_exit "非法编号格式: $item" 10
        clean_arr+=("$num")
    done
    echo "${clean_arr[@]}"
}

game() {
    current_ip=$(get_host_name "$line_num")
    [[ ! -f "${gameVars}/main.yml.tmp" ]] && error_exit "模板文件不存在" 8

    # 获取并处理数组
    items=($(to_arr_fun "$line_num")) || exit $?
    game_port_index=0

    for item in "${items[@]}"; do
        server_num=$item
        game_port=$((game_port_start + game_port_index * 10))
        
        echo "当前正在配置：行号=$line_num | IP=$current_ip | 端口=$game_port | 编号=$server_num"
        
        # 生成配置文件
        export current_ip game_port server_num
        envsubst < "${gameVars}/main.yml.tmp" > "${gameVars}/main.yml" || error_exit "配置文件生成失败" 9

        # 执行Ansible
        ansible-playbook -i "${current_ip}," \
            -e "host_name=${current_ip}" \
            -e "role_name=game" \
            "${playbookFile}" || error_exit "Ansible任务失败，任务名：game，server_num编号: $server_num" 14

        ((game_port_index++))
    done
}

gate() {
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

login() {
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

main() {
    case "$1" in
        game)
            game
            ;;
        login)
            login
            ;;
        gate)
            gate
            ;;
        all)
            game
            gate
            login
            ;;
        *)
            usage
            ;;
    esac
}

main "$install_type"