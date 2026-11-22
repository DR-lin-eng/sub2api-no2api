# Gemini CLI

Gemini CLI 通过 Gemini 原生接口工作，需要 Gemini 或 Antigravity 分组的 Key。

## 配置

`GOOGLE_GEMINI_BASE_URL` 填站点根地址，CLI 会自己追加 `/v1beta`。

::: code-group

```bash [macOS / Linux]
export GOOGLE_GEMINI_BASE_URL="https://api.example.com"
export GEMINI_API_KEY="sk-your-api-key"
export GEMINI_MODEL="your-model"
```

```powershell [Windows PowerShell]
$env:GOOGLE_GEMINI_BASE_URL="https://api.example.com"
$env:GEMINI_API_KEY="sk-your-api-key"
$env:GEMINI_MODEL="your-model"
```

```bat [Windows CMD]
set GOOGLE_GEMINI_BASE_URL=https://api.example.com
set GEMINI_API_KEY=sk-your-api-key
set GEMINI_MODEL=your-model
```

:::

设置后在同一个终端里运行 `gemini`。`GEMINI_MODEL` 填 Gemini 格式[模型列表](/api/models#gemini-格式)返回的模型名。

## Antigravity 分组

Antigravity 分组的 Key 把地址改为专用前缀：

```bash
export GOOGLE_GEMINI_BASE_URL="https://api.example.com/antigravity"
```

## 排查

| 现象 | 处理 |
| --- | --- |
| 提示 401 | 检查 `GEMINI_API_KEY` |
| 提示 404 或模型不可用 | 确认 Key 属于 Gemini 或 Antigravity 分组，且模型名在列表中 |
