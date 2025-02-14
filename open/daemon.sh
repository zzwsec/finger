#!/bin/bash

host=192.168.121.120
port=3306
user=root
password=root
workDir="/data/ansible/open"
init_file="${workDir}/init.txt"
log_file="${workDir}/info.log"
dataBase="cbt4_log"

# 如果 init.txt 不存在，则创建并初始化
if [[ ! -f "$init_file" ]]; then
    echo "1" > "$init_file"
fi

while :; do
    run_svc_num=$(sed -n '$p' "$init_file")
    next_run_svc_num=$((run_svc_num + 1))

    sql="SELECT COUNT(id) FROM log_register WHERE zone_id=${run_svc_num}"
    next_sql="SELECT COUNT(id) FROM log_register WHERE zone_id=${next_run_svc_num}"

    # 执行 MySQL 查询
    count_run_svc_num=$(mysql -u"$user" -p"$password" -h "$host" -P "$port" "$dataBase" -e "$sql" -s -N 2>/dev/null)
    if [ $? -ne 0 ]; then
        echo "数据库连接失败，退出" >> "$log_file"
        exit 1
    fi
    count_run_svc_next_num=$(mysql -u"$user" -p"$password" -h "$host" -P "$port" "$dataBase" -e "$next_sql" -s -N 2>/dev/null)

    # 条件判断
    if [[ -n "$count_run_svc_num" ]] && [[ "$count_run_svc_num" -ge 1000 ]]; then
        if [[ -n "$count_run_svc_next_num" ]] && [[ "$count_run_svc_next_num" -eq 0 ]]; then
            if cd "$workDir"; then
                if ./install.sh "$next_run_svc_num" 1 &>/dev/null; then
                    echo "$next_run_svc_num" > "$init_file"
                    echo "[SUCCESS] 开启服务编号为：${next_run_svc_num}，开启成功" >> "$log_file"
                else
                    echo "[ERROR] 开启失败，服务编号: ${next_run_svc_num}" >> "$log_file"
                    exit 2
                fi
            else
                echo "[ERROR] 无法进入目录：$workDir" >> "$log_file"
                exit 3
            fi
        else
            echo "[ERROR] 当前服务编号：${run_svc_num}，下一服务数量：${count_run_svc_next_num}。下一个服务中人数不为空，退出" >> "$log_file"
            exit 4
        fi
    else
        echo "[INFO] 当前服务编号：${run_svc_num}，数量：${count_run_svc_num}。未达到阈值" >> "$log_file"
    fi
    sleep 30
done

