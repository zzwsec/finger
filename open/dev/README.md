# CDN 测试服务

该服务模拟 `open` 所调用的 CDN 接口，不执行真实的 CDN 刷新。

启动：

```bash
cd open
go run ./dev
```

默认监听 `:20011`，也可以通过环境变量修改：

```bash
DEV_CDN_ADDRESS=:8080 go run ./dev
```

测试：

```bash
curl 'http://127.0.0.1:20011/openserver/?zone_id=60'
```

成功响应：

```json
{"code":0,"message":"success"}
```

如果测试服务与 `open` 容器不在同一个容器中，`OPEN_CDN_URL` 需要填写测试服务所在主机可被容器访问的 IP，不能使用容器自身的 `127.0.0.1`。
