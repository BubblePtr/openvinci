# GitHub Releases + 一行 install.sh 作为 Agent 分发面

分发规格在立项时已定为 GitHub Releases + `curl | sh`，平台 macOS arm64 与 Linux x64/arm64。实现上用 GoReleaser 在 `v*` tag 上交叉编译静态二进制，`install.sh` 放在仓库根目录，作为和 `vinci --help` 同级的安装契约：Agent 不需要 Go，不需要猜测资产文件名。

安装默认写入 `~/.local/bin`，避免 sudo。版本号由 ldflags 写入 `main.version`。Windows、Homebrew、公证不在第一版。

## Consequences

- 发版动作是在 `main` 上打 annotated tag 并 push；没有 tag 就不会出包。
- `install.sh` 在尚无 Release 时会失败，错误信息指向需要先打 tag。
- README 的默认安装路径从 `go build` 改为 `curl | sh`。
