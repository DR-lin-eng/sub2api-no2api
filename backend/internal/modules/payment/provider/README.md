# Payment Providers

每个文件实现一个外部支付渠道适配器：`stripe.go`, `alipay.go`, `wxpay.go`, `airwallex.go`, `easypay.go`。`factory.go` 根据配置构造适配器。

适配器负责签名、请求和响应规范化，不负责订单状态机。新增渠道必须同时提供回调验签和错误映射测试。

EasyPay 提供可选的 `async` 模式，通过服务端 `mapi.php` 下单；原有 `popup` 的 `submit.php` 同步跳转、`qrcode` 与历史默认 API 行为保持。`NotificationConfirmer` 通过同一实例的订单查询确认已支付状态、金额和交易号；查询兼容 PHP query/form 参数与字符串标量，传输错误不包含带商户密钥的 URL。HTTPS、loopback 开发例外和禁止 HTTP 重定向策略保持。

Alipay 退款保存 `out_request_no` 作为可查询 ID，使用审计创建时间执行十分钟无记录宽限。旧交易号格式不能推断退款请求号，返回明确的人工核实错误；业务失败或网络错误保留 pending。
