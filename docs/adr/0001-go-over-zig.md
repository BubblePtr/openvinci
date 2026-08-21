# 用 Go 而非 Zig 实现单二进制 CLI

最初计划用 Zig 以兼顾学习目标，但调研（2026-08）显示 Zig 0.16 的 `std.http.Client` + 纯 Zig TLS 不具生产可用性：POST 发送失败（ziglang/zig#25002）、大 payload HTTPS 挂起（#25015）、部分服务器握手失败，且标准库 API 每版本破坏性变更；可靠方案需绑 libcurl（破坏零运行时依赖卖点）或引入第三方 TLS 实现。产品目标（Agent 稳定调用 + 零依赖单二进制）用 Go 原生达成，交叉编译与分发（goreleaser）成熟，故放弃 Zig。

## Consequences

- 学习 Zig 的目标从本项目移除。
- "零运行时依赖单二进制"由 Go 静态编译保证，分发平台为 macOS arm64、Linux x64/arm64（Windows 延后）。
