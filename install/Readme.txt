step1:
    name: 检查 install_list/game_list.txt
    tips: 第一列为ip，第二列为当前主机运行的服务，第三列为group_id

step2:
    name: 检查 roles/game/vars/main.yml
    tips: 检查数据库地址、kingnet回调地址、zk的ip、domain

step3:
    name: 检查roles/game/templates/
    tips: 检查server.app.lua.j2和zones.lua.j2文件

step4:
    name: roles/game/files/etc/
    tips: 该目录存放安装后的etc目录中的文件

step5:
    name: roles/game/files/install.zip
    tips: 确保该安装包解压后能直接得到lua、p8_app_server等文件和目录（没有多余的目录）

step6:
    name: 启动
    tips: bash game.sh