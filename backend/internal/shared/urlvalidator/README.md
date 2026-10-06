# urlvalidator

上游 URL/SSRF 安全校验。文件索引：`validator.go` 及测试；`public.go` 为用户提交的图片 URL 提供独立的公开地址策略。

`ValidatePublicHTTPURL` 校验协议、凭据和字面量地址；调用方仍须对 DNS 结果逐一执行 `ValidatePublicIP`，并将已验证 IP 固定为连接目标，重定向的每一跳都重新校验。管理员的私网上游例外不得放宽用户图片策略。
