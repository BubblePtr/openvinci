# 如何发版

OpenVinci 只在 git tag 上出包。日常 push 不会发 GitHub Release。

## 切一个版本

1. 确认要发布的提交已在 `main`，工作区干净，`go test ./...` 与 `sh install_test.sh` 通过。
2. 打 annotated tag，版本号遵循 SemVer，带 `v` 前缀：

```bash
git tag -a v0.1.0 -m "v0.1.0"
git push origin v0.1.0
```

3. GitHub Actions 的 `release` workflow 会跑 GoReleaser，上传：

- `vinci_darwin_arm64.tar.gz`
- `vinci_linux_amd64.tar.gz`
- `vinci_linux_arm64.tar.gz`
- `checksums.txt`

4. 打开 GitHub Releases，确认 `vinci --version` 在解压后等于 tag（不含前缀处理以 GoReleaser 写入的 `{{.Version}}` 为准，一般为 `0.1.0`）。
5. 在一台目标机器上试装：

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh
vinci --version
```

钉死版本：

```bash
curl -fsSL https://raw.githubusercontent.com/BubblePtr/openvinci/main/install.sh | sh -s -- --version v0.1.0
```

## 不要做什么

- 不要从功能分支打 tag。
- 不要手搓 Release 资产；资产必须来自 workflow，否则 checksum 对不上 `install.sh`。
- 不要为 Windows 或 macOS Intel 加目标，除非先改 ADR-0001 与 `install.sh` 的平台表。
