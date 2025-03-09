step1:
    name: 检查file目录
    tips: groups.lua 和 increment.zip 不能共存，只能有一个在 file 目录中

step2:
    name: 检查 increment.zip 解压产物
    tips: 确保 increment.zip 解压后能得到一个 app 目录

step3:
    name: 检查 hosts 文件中各个服务的 ip 是否正确

step4:
    name: 启动
    tips: bash start.sh