#!/bin/bash

# 参数验证与赋值
if [ $# -eq 1 ]; then
    server_num=$1
elif [ $# -eq 2 ]; then
    server_num=$1
    white=$2
else
    echo -e "使用方法：\n
    第一个参数为 server 编号（必填），\n
    第二个为白名单，非空表示开启白名单，为空表示不开启（默认）\n"
    exit 1
fi


get_host_name() {
    local line_num=$1
    awk '{print $1}' < <(sed -n "${line_num}p" list.txt)
}

# 获取当前时间
time=$(date +%FT%H:00:00)
sed -ri 's#^(\s*open_server_time\s*=).*#\1 "'"${time}"'"#' ./roles/open/files/open_time.lua

row=0  # 当前节点行号
found=0 # 标记是否找到匹配项

# 判断输入的服务器编号是否已存在
while read -r line; do
    echo "当前处理的行内容：$line"
    let row++
    # 解析中括号内的数字并逐一匹配
    for i in $(echo "$line" | awk -F'[][]' '{print $2}' | tr ',' ' '); do
        if [ "$server_num" -eq "$i" ]; then
            found=1
            echo "找到匹配项：$1，服务器行号为$row"
            break 2  # 如果找到，直接退出外层循环
        fi
    done
done < list.txt

echo "当前行号row: $row"
echo "标志位为: $found"

# 如果未找到匹配项
if [ $found -eq 0 ]; then
    echo "输入的服务编号未找到：$1"
    exit 1
fi

# 将指定的行转换为数组
to_arr_fun() {
    local line_num=$1
    local line
    line=$(sed -n "${line_num}p" list.txt)
    arr=($(echo "$line" | awk -F'[][]' '{print $2}' | tr ',' ' '))
    echo ${arr[@]}
}

# 更新白名单
white_fun() {
    if [ -n "$white" ]; then
        local line_num=$1
        local host_name=$(sed -n "${line_num}p" list.txt | awk '{print $1}')
        echo "当前在更新白名单的函数中，server_num：$server_num"
        ansible-playbook -i "${host_name}," -e "host_name=$host_name" -e "role_name=white" -e "white_num=$server_num" example.yaml
        if [ $? -ne 0 ];then
            echo "white剧本执行失败，返回1"
            exit 1
        fi
    fi
}

# 更新limit名单
limit_fun() {
    echo "当前在更新limit的函数中"
    local line_num=$1
    local svc_num=$2
    local host_name=$(sed -n "${line_num}p" list.txt | awk '{print $1}')
    #清空文件
    if [ -f ./roles/limit/files/limit_create.txt ]; then
        : >./roles/limit/files/limit_create.txt
    fi
    for i in $(to_arr_fun $line_num); do
	    echo "$i"
	    #相等表示终止
        if [ $i -eq $svc_num ]; then
		    echo "相等表示终止,终止svc: $svc_num"
       		if [ $line_num -ne $row ];then
			echo $i >> ./roles/limit/files/limit_create.txt
		    fi
            break
        else
            echo $i >> ./roles/limit/files/limit_create.txt
        fi
    done
    echo "执行limit剧本"
    ansible-playbook -i "${host_name}," -e "host_name=$host_name" -e "role_name=limit" example.yaml
	if [ $? -ne 0 ];then
		echo "limit剧本执行失败，返回2"
		exit 2
	fi
}

# 开启服务
open_fun() {
    echo "当前在更新open的函数中"
    local line_num=$1
    local svc_num=$2
    local host_name=$(sed -n "${line_num}p" list.txt | awk '{print $1}')
    echo "ip地址为：$host_name"
    ansible-playbook -i "${host_name}," -e "host_name=$host_name" -e "role_name=open" -e "svc_num=$svc_num" example.yaml
    if [ $? -ne 0 ];then
    	echo "open剧本执行失败，返回3"
    	exit 3
    fi
}

# 删除日志
remove_log() {
    local line_num=$1
    local svc_num=$2
    local host_name
    host_name=$(get_host_name "$line_num")
    echo "ip地址：$host_name"

    ansible -i "${host_name}," "${host_name}" -m shell -a "rm -rf /data/server${svc_num}/game/log/*"
    if [ $? -ne 0 ];then
    	echo "日志删除执行失败，返回4"
    	exit 4
    fi
}

# 获取当前行的数组
p_arr=($(to_arr_fun "$row"))
first_element="${p_arr[0]}"

echo "第一个元素是: $first_element"
# 更新白名单
white_fun "$row"

# 更新open服务
open_fun "$row" "$server_num"

## 根据条件执行limit更新
if [ "${first_element}" -eq "$server_num" ]; then
    row_tmp=$((row - 1))
    arr_tmp=($(to_arr_fun "$row_tmp"))
    limit_fun "$row_tmp" "${arr_tmp[-1]}"
else
    limit_fun "$row" "$server_num"
fi

# 删除日志
remove_log "$row" "$server_num"
