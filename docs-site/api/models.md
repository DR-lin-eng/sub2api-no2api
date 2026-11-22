# 模型列表

<Endpoint method="GET" path="/v1/models" />

返回当前 Key 所属分组可以调用的模型。模型 ID 由站点管理员配置，不同站点、不同分组的结果不同，请以这个接口的返回为准。

## 请求

```bash
curl -sS "https://api.example.com/v1/models" \
  -H "Authorization: Bearer $SUB2API_API_KEY"
```

## 响应

```json
{
  "object": "list",
  "data": [
    {"id": "your-model", "object": "model", "owned_by": "openai"}
  ]
}
```

每个条目至少包含 `id`。其余字段随分组平台变化，可能带有显示名称和能力信息。

## 查询单个模型

<Endpoint method="GET" path="/v1/models/{model}" />

返回列表中对应的完整条目。模型不存在或当前分组不可用时返回 404：

```json
{
  "error": {
    "type": "invalid_request_error",
    "code": "model_not_found",
    "param": "model",
    "message": "Model \"foo\" does not exist or is not available for this group"
  }
}
```

## Gemini 格式

<Endpoint method="GET" path="/v1beta/models" />

Gemini 客户端使用这组路径，返回 Gemini API 的模型列表结构，顶层字段为 `models`。单个模型用 `GET /v1beta/models/{model}` 查询。

```bash
curl -sS "https://api.example.com/v1beta/models" \
  -H "x-goog-api-key: $SUB2API_API_KEY"
```

## 模型别名

管理员可以为分组配置对外的模型别名。请求时使用列表返回的 ID，网关会在转发时改写为上游的真实模型名。复合分组还会根据模型名决定把请求路由到哪个平台，说明见仓库的[复合分组文档](https://github.com/DR-lin-eng/sub2api-no2api/blob/main/docs/COMPOSITE_GROUPS.md)。
