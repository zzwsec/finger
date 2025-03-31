停止：
  bash control.sh stop

启动：
  bash control.sh start

=============================
单独停止：
  process:
    - ansible-playbook playbook/process/process-entry.yaml -t 'stop'
  login:
    - ansible-playbook playbook/login/login-entry.yaml -t 'stop'
  gate:
    - ansible-playbook playbook/gate/gate-entry.yaml -t 'stop'
  game:
    - ansible-playbook playbook/game/game-entry.yaml -t 'stop'
  cross:
    - ansible-playbook playbook/cross/cross-entry.yaml -t 'stop'
  gm:
    - ansible-playbook playbook/gm/gm-entry.yaml -t 'stop'
  global:
    - ansible-playbook playbook/global/global-entry.yaml -t 'stop'
  log:
    - ansible-playbook playbook/log/log-entry.yaml -t 'stop'
  zk:
    - ansible-playbook playbook/zk/zk-entry.yaml -t 'stop'

启动：
  zk:
    - ansible-playbook playbook/zk/zk-entry.yaml -t 'start'
  log:
    - ansible-playbook playbook/log/log-entry.yaml -t 'start'
  global:
    - ansible-playbook playbook/global/global-entry.yaml -t 'start'
  cross:
    - ansible-playbook playbook/cross/cross-entry.yaml -t 'start'
  gm:
    - ansible-playbook playbook/gm/gm-entry.yaml -t 'start'
  game:
    - ansible-playbook playbook/game/game-entry.yaml -t 'start'
  gate:
    - ansible-playbook playbook/gate/gate-entry.yaml -t 'start'
  login:
    - ansible-playbook playbook/login/login-entry.yaml -t 'start'
  process:
    - ansible-playbook playbook/process/process-entry.yaml -t 'start'