# Claude Code

Claude Code 通过 Anthropic Messages 接口工作。把它的 API 地址指向你的 Sub2API 站点即可。

## 配置

`ANTHROPIC_BASE_URL` 填站点根地址，不带 `/v1`。

::: code-group

```bash [macOS / Linux]
export ANTHROPIC_BASE_URL="https://api.example.com"
export ANTHROPIC_AUTH_TOKEN="sk-your-api-key"
export CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

```powershell [Windows PowerShell]
$env:ANTHROPIC_BASE_URL="https://api.example.com"
$env:ANTHROPIC_AUTH_TOKEN="sk-your-api-key"
$env:CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

```bat [Windows CMD]
set ANTHROPIC_BASE_URL=https://api.example.com
set ANTHROPIC_AUTH_TOKEN=sk-your-api-key
set CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1
```

:::

设置后在同一个终端里运行 `claude`。

## 写入配置文件

想让配置长期生效，或在 VS Code 扩展中使用，把它写进 `~/.claude/settings.json`（Windows 为 `%USERPROFILE%\.claude\settings.json`）：

```json
{
  "$schema": "https://json.schemastore.org/claude-code-settings.json",
  "env": {
    "ANTHROPIC_BASE_URL": "https://api.example.com",
    "ANTHROPIC_AUTH_TOKEN": "sk-your-api-key",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

## 使用非 Claude 模型

Key 属于非 Anthropic 分组时，网关会把 Messages 请求转换为对应平台的协议。此时需要告诉 Claude Code 使用哪个模型：

```bash
export ANTHROPIC_MODEL="your-model"
export ANTHROPIC_DEFAULT_OPUS_MODEL="your-model"
export ANTHROPIC_DEFAULT_SONNET_MODEL="your-model"
export ANTHROPIC_DEFAULT_HAIKU_MODEL="your-model"
```

模型 ID 用[模型列表](/api/models)查询。

## Antigravity 分组

Antigravity 分组的 Key 把地址改为专用前缀：

```bash
export ANTHROPIC_BASE_URL="https://api.example.com/antigravity"
```

## 排查

| 现象 | 处理 |
| --- | --- |
| 提示 401 | 检查 `ANTHROPIC_AUTH_TOKEN` 是否完整，Key 是否被停用 |
| 提示 404 | 确认 `ANTHROPIC_BASE_URL` 没有多写 `/v1` |
| 提示模型不存在 | 分组里没有 Claude Code 默认请求的模型，按上文指定模型 |

::: tip
站点控制台的「API 密钥 → 使用密钥」会按你的 Key 和分组生成同样的配置，可以直接复制。
:::
