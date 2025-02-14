#!/bin/bash

host=192.168.121.120
port=3306
user=root
password='root'
dataBase=cbt4_log
initFile="/data/ansible/open/init.txt"
logFile="/data/ansible/open/info.log"

while true; do
    current_time=$(date "+%F %T")

    if [ ! -s "$initFile" ]; then
        echo "[ERROR] ${current_time}: $initFile 不存在或为空" >> "$logFile"
        exit 1
    fi

    # 获取当前服务编号
    current_svc_num=$(tail -n 1 "$initFile")
    if [[ ! $current_svc_num =~ ^[0-9]+$ ]]; then
        echo "[ERROR] ${current_time}: init.txt 内容非法" >> "$logFile"
        exit 2
    fi

    # 获取下一个服务编号玩家数
    next_svc_num=$((current_svc_num + 1))

    current_sql="SELECT COUNT(*) FROM ${dataBase}.log_register WHERE zone_id=${current_svc_num}"
    next_sql="SELECT COUNT(*) FROM ${dataBase}.log_register WHERE zone_id=${next_svc_num}"

    echo "[DEBUG] ${current_time}: current_sql=${current_sql}" >> "$logFile"
    echo "[DEBUG] ${current_time}: next_sql=${next_sql}" >> "$logFile"

    current_svc_count=$(mysql -u"$user" -p"$password" -h "$host" -P "$port" -e "${current_sql}" -s -N 2>/dev/null)
    echo "[DEBUG] ${current_time}: current_svc_count=${current_svc_count}" >> "$logFile"

    if [ $? -ne 0 ]; then
        echo "[ERROR] ${current_time}: 查询当前服务编号: ${current_svc_num} 失败" >> "$logFile"
        exit 3
    fi

    next_svc_count=$(mysql -u"$user" -p"$password" -h "$host" -P "$port" -e "${next_sql}" -s -N 2>/dev/null)
    echo "[DEBUG] ${current_time}: next_svc_count=${next_svc_count}" >> "$logFile"

    if [ $? -ne 0 ]; then
        echo "[ERROR] ${current_time}: 查询下一个服务 ${next_svc_num} 失败" >> "$logFile"
        exit 4
    fi

    if [[ -z "${next_svc_count}" ]] || [[ "${next_svc_count}" -ne 0 ]]; then
        echo "[ERROR] ${current_time}: 下一个服务 ${next_svc_num} 已有玩家数据, 异常数据" >> "$logFile"
        exit 4
    fi

    if [[ "${current_svc_count}" -ge 1000 ]]; then
        echo "[INFO] ${current_time}: 满足扩容条件，开始部署 ${next_svc_num}" >> "$logFile"

        if cd /data/ansible/open && ./install.sh "$next_svc_num" 1; then
            echo "$next_svc_num" > "$initFile"
            echo "[SUCCESS] ${current_time}: 成功更新服务编号至 ${next_svc_num}" >> "$logFile"
        else
            echo "[ERROR] ${current_time}: 服务 ${next_svc_num} 部署失败" >> "$logFile"
            exit 5
        fi
    else
        echo "[INFO] ${current_time}: 当前服务 ${current_svc_num} 玩家数 ${current_svc_count} 未达条件" >> "$logFile"
    fi

    sleep 30
done
