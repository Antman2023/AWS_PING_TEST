# AWS_PING_TEST

从 AWS 官方 EC2 Reachability JSON 源拉取测试目标，并对目标地址执行 `ping`，输出低延迟节点列表。

## 用法

```bash
go run . -region ap-northeast-1,ap-southeast-1
```

常用参数：

- `-family`: 地址族，`ipv4` 或 `ipv6`，默认 `ipv4`
- `-region`: 仅测试指定区域，多个区域使用逗号分隔
- `-count`: 每个目标发送的 ping 次数，默认 `3`
- `-timeout`: 单个目标总超时时间，默认 `5s`
- `-interval`: 发包间隔，默认 `200ms`
- `-parallel`: 并发测试目标数，默认 `12`
- `-top`: 只展示延迟最低的前 N 个可达节点，默认 `20`，设为 `0` 表示全部展示
- `-privileged`: 是否使用原始套接字模式，Windows 默认启用

示例：

```bash
go run . -region ap-northeast-1 -count 2 -timeout 3s -top 10
go run . -family ipv6 -region us-east-1,us-west-2 -parallel 6
```

## 构建

```bash
go build .
./build.sh
```

`build.sh` 会输出多平台二进制到 `release/` 目录。
