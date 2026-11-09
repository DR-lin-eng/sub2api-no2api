# GPT Image 2.6 SVG/GIF 验证（2026-11-09）

## 官方能力与实现

依据实际读取的 OpenAI [图像生成指南](https://developers.openai.com/api/docs/guides/image-generation)、[Sunburst 模型页](https://developers.openai.com/api/docs/models/gpt-image-2.6-sunburst) 和 [Images 生成接口](https://developers.openai.com/api/reference/resources/images/methods/generate)：Flare 和 Sunburst 均支持 SVG、动画 GIF。SVG 仅支持生成，单次 `n=1`；GIF 支持生成/编辑，`frame_count` 为 2–16、默认 9，帧时长默认 200 ms。两者均不支持流式输出，实际账号仍需要上游对应权限。

账号画图测试新增格式、GIF 帧数/时长及可选 SVG 作者与推理设置。API Key/OAuth 采用同一参数合同；测试会解析实际 SVG XML 或 GIF 文件块，错误格式和单帧 GIF 不会被确认为对应能力。预览和下载保留正确 MIME/后缀。普通 ChatGPT App 通道明确提示原生格式不可用，不静默换协议。

媒体工坊的原生 2.6 分组提供格式选择，参数保留到请求、历史、刷新和重试，并服从请求模板覆盖。SVG 固定单文件生成，不作为 2.6 编辑参考图；GIF 使用 `size=auto`，按自然比例显示，避免多帧像素预算和裁切问题。自动尺寸尚不能确定计费档位时不臆测金额。管理员需要按现有媒体工坊配置声明 2.6 图片能力并开放对应分组/模型；ChatGPT App 分组隐藏原生格式选项。独立调度的健康停调原生账号也参与协议能力发现，错误账号不参与。

## 计费规则

开启分组“按实际生图张数计费”后，GIF 累计实际完成动画的帧数；优先读取文件帧数，其次读取逐结果/响应级 `frame_count`，再使用请求值或官方默认 9。b64_json 和 data-URL 两种表示使用相同规则，尺寸档位按帧累计。没有返回文件的 SVG/GIF 不提交成功结果，也不计费。

SVG 按一个作品结算；按张/按次价格覆盖完整 SVG，不额外追加作者 Token 费用。Token 模式仍分别计算参考图片及 SVG 作者的实际用量。关闭实际张数计费时，不额外把 GIF 动画数量或 Token 用量乘以帧数。沿用原有可靠结算命令与幂等机制，无数据库迁移。

## Docker 验证

- 前端 10 个文件、69 项测试通过；typecheck、lint 和生产构建通过。
- 后端图片协议、账号测试、SVG/GIF 文件校验、帧计费、SVG 作者结算、媒体工坊预估和 HTTP 契约相关回归通过。额外覆盖空结果不计费、错误文件不报成功、URL 表示与 b64 表示一致，以及健康停调账号的原生能力发现。
- 完整镜像：`sub2api:image26-svg-gif-20261109`，最终镜像 ID 为 `sha256:b93fead53b3c275d8c704a88462076b99986e56bdd27536d79d4fd063daeed83`。
- 使用 PostgreSQL 18、Redis 8、标准计费模式及本地模拟上游实际执行，模拟上游返回标准编码的多帧 GIF 和 SVG XML，不调用真实收费上游。

测试配置单价 $0.25/张、分组倍率 2：

| 输出 | 使用记录 image_count | 原始费用 | 实扣费用 | 记录输入/输出 Token |
| --- | --- | --- | --- | --- |
| 4 帧 GIF | 4 | $1.00 | $2.00 | 10 / 20 |
| SVG | 1 | $0.25 | $0.50 | 60 / 40（含作者用量） |

测试余额从 $100.00 结算到 $97.50，与预期一致；GIF Token 没有按帧重复放大。精确交付镜像在全新 PostgreSQL/Redis 实例上重复通过了同一文件与扣费矩阵。固定 1K 尺寸的 4 帧 GIF 预估为 $2.00，与结算一致；自动尺寸明确返回预估不可用。

Playwright 在实际 Docker 页面完成两种格式的生成和预览，作品库实际下载得到 `.svg` 与 `.gif`；下载文件分别通过 XML 根节点和 `GIF89a` 文件头验证。页面刷新后恢复 SVG 格式及两份已生成作品，图片自然宽度分别为 96、64，均已加载。390px 手机宽度下页面宽度为 382px，没有横向溢出；GIF 帧数/时长控件与计费说明显示正常。

`make check-docs` 与 `git diff --check` 通过。源码布局检查仍被本次未修改的基线文件 `payment_fulfillment_test.go`（1201 行，限制 1200）阻挡。未进行生产部署或真实账号预览权限测试；实际权限可用新增账号画图测试验证。临时测试容器与网络在验收后清理。
