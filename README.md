# gproxy

http/https 调试工具,可以用来取代 Charles

## 特性
- [ ] http (pure) proxy
- [ ] https proxy
- [ ] https rootca
- [ ] request/response filter
- [ ] 请求录制和重放
- [ ] snapshot 快照
- [ ] 限速,模拟2G/3G/4G
- [ ] load-balancing(可用于反向代理) 
- [ ] websocket frames
- [ ] h2 stream view
- [ ] socks5
- [ ] webui [anyproxy-ui](https://github.com/alibaba/anyproxy/tree/master/web) [devtools-frontend](https://github.com/ChromeDevTools/devtools-frontend)
- [ ] local agent proxy(通过pure proxy可以支持pac)

## Build from source

```bash
cd cmd

go build -o gproxy
```

## 使用

开发中...

```bash

# proxy 
./gproxy

# https (参与握手) proxy
# 创建证书
./gproxy cert -host letsencrypt.org test
./gproxy -host letsencrypt.org -key test.ket -cert test.cert

# test
curl --cacert ./test-ca.cert -v --proxy http://127.0.0.1:8080  https://letsencrypt.org/test


# 如果只是一个单纯的 http proxy (目前还不稳定)
./gproxy pure

```
