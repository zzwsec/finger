step1:
    name: 检查file目录
    tips: groups.lua 和 increment.tar.gz 不能共存，只能有一个在 file 目录中

step2:
    name: 检查 increment.tar.gz 解压产物
    tips:
    - 确保 increment.tar.gz 解压后能得到一个 app 目录
    - 压缩方式：mkdir app && find . -maxdepth 1 -not -name "app" -not -name "." -exec cp -r {} app/ \; && tar -zcvf increment.tar.gz app

step3:
    name: 检查 hosts 文件中各个服务的 ip 是否正确

step4:
    name: 启动
    tips: bash start.sh

====================================================================
单独执行剧本使用如下方式：

更新groups:
  - cross: ansible-playbook playbook/cross/cross-entry.yaml -t groups
  - game: ansible-playbook playbook/game/game-entry.yaml -t groups

其他类型更新:
  - cross: ansible-playbook playbook/cross/cross-entry.yaml -t increment
  - game: ansible-playbook playbook/game/game-entry.yaml -t increment
  - gm: ansible-playbook playbook/gm/gm-entry.yaml
  - log: ansible-playbook playbook/log/log-entry.yaml
