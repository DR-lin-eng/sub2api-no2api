# claude

Claude 协议常量位于 `constants.go`。`cli_version.go` 在进程启动时读取可选的 `SUB2API_CLAUDE_CLI_VERSION`；空值沿用内置版本，仅接受不低于内置 pin 的稳定三段 semver。修改环境变量后需重建容器/重启进程；移除变量即可恢复内置值。
