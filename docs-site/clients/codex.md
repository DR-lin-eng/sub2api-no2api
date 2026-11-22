# Codex CLI

Codex CLI 通过 OpenAI Responses 接口工作。在它的配置文件里添加一个指向 Sub2API 的模型提供方即可。

## 配置

编辑 `~/.codex/config.toml`（Windows 为 `%USERPROFILE%\.codex\config.toml`）：

```toml
model_provider = "sub2api"
model = "your-model"

[model_providers.sub2api]
name = "OpenAI"
base_url = "https://api.example.com"
wire_api = "responses"
env_key = "OPENAI_API_KEY"
requires_openai_auth = false
```

然后在终端里提供 Key，再运行 `codex`：

::: code-group

```bash [macOS / Linux]
export OPENAI_API_KEY="sk-your-api-key"
```

```powershell [Windows PowerShell]
$env:OPENAI_API_KEY="sk-your-api-key"
```

:::

## 说明

- `base_url` 填站点根地址。Codex 会请求 `{base_url}/responses`，网关为它提供了不带 `/v1` 的别名。填 `https://api.example.com/v1` 也可以。
- `wire_api` 必须是 `responses`。
- `env_key` 让 Codex 从环境变量读取 Key，Key 不会写进配置文件。
- `model` 填[模型列表](/api/models)返回的模型 ID。

## 排查

| 现象 | 处理 |
| --- | --- |
| 提示 401 | 当前终端没有设置 `OPENAI_API_KEY`，或 Key 无效 |
| 提示 404 | 检查 `base_url` 是否拼错，以及 Key 的分组是否支持 Responses |
| 仍然连到官方服务 | 确认 `model_provider` 的值与 `[model_providers.xxx]` 的名字一致 |

::: tip
站点控制台的「API 密钥 → 使用密钥」会生成包含推荐模型和附加选项的完整配置。
:::
