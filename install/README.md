step1:
    name: 检查 install_list/game_list.txt
    tips:
      - 第一列为ip，第二列为当前主机运行的服务，第三列为 group_id
      - 该文件需要 dos2unix 处理一下

step2:
    name: 检查 roles/game/vars/main.yml
    tips: 检查数据库地址、kingnet 回调地址、zk 的 ip、domain

step3:
    name: 检查roles/game/templates/
    tips: 检查 server.app.lua.j2 和 zones.lua.j2 文件

step4:
    name: 启动
    tips: bash game.sh 服务编号 [是否启动，默认不启动，如果要启动则传入 start]
