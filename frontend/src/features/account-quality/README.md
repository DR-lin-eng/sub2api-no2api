# Admin Account Quality

账号质量巡检独立管理质量探测策略、分组切换、手动运行和结果快照。

- `data/dtos/accountQualityDtos.ts`: 质量设置、运行摘要和结果协议。
- `data/datasources/accountQualityDatasource.ts`: `/admin/account-quality` 请求 owner。
- `presentation/pages/AccountQualityPage.vue`: 质量设置、摘要和结果表。

管理页操作区提供“打开公开展示页”按钮，跳转到 `/monitor/quality/public`；手动巡检会立即返回运行状态，页面每 5 秒显示总任务完成/执行/排队数量、逐账号阶段和耗时，已完成的结果增量出现；轮询不覆盖正在编辑的设置。第一阶段等待和第二阶段无输出等待默认 120 秒，可在质量巡检策略中自定义 30–300 秒。第二阶段使用 OpenAI 流式探测，收到内容/图片输出后不再触发这项短超时，让长时间画图自然完成。质量巡检分为两个可独立开关的阶段：第一阶段是糖果形状/口味保证题（默认答案 21），第二阶段是 SVG 鹈鹕骑自行车画图题，按 9 项 HTML/SVG 代码特征匹配 Model A；管理员可设置匹配阈值及命中/未命中为正常的规则。前端浏览器在 sandboxed iframe 中渲染 HTML/SVG，历史记录保留 WebP 回退，预览失败不影响代码匹配结果。管理员可编辑第一阶段题目与答案、第二阶段提示词。页面显示每个账号的阶段结果、实际 reasoning token 和完整分布；低于阈值的账号标记为降智，缺失 token 显示为待确认。

质量设置使用 `account_quality_settings`，运行状态使用 `account_quality_state`；账号质量巡检只筛选启用的 OpenAI/Gemini OAuth 账号，API Key 和 service account 永不进入检测队列；账号健康巡检使用另一组设置、状态和调度器。


第二阶段 HTML/SVG 在 sandboxed iframe 中由前端浏览器渲染，历史记录保留 WebP 图片回退。
