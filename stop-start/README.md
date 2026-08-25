## 配置主机

编辑 `hosts`，先统一定义主机，再将主机别名加入对应服务组：

```ini
[all]
node-01 ansible_host=192.168.121.102
node-02 ansible_host=192.168.121.103

[game]
node-01
node-02

[login]
node-01
```

同一台主机可以加入多个服务组，IP 只需要在 `[all]` 中维护一次。

## 批量启停

```bash
cd stop-start
bash control.sh stop
bash control.sh start
```

启动顺序：

```text
zk → log → global → cross → gm → game → gate → login
```

停止顺序：

```text
login → gate → game → cross → gm → global → log → zk
```

`game`、`cross`、`gm`、`global`、`log` 停止时每批处理两台主机。同一主机上的
unit 使用 no-block 快速提交停止请求，当前批次全部停止后才会继续下一批。

## 单独启停

```bash
bash control.sh stop game
bash control.sh start game
```

服务类型支持 `game`、`login`、`gate`、`cross`、`gm`、`global`、`log` 和 `zk`。

playbook 直接查找 systemd unit，不依赖程序目录结构。未找到对应 unit 时执行失败。

执行中途失败时日志保留在 `runlog/`，全部执行成功后自动删除该目录。
