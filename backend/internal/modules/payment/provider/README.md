# Payment Providers

每个文件实现一个外部支付渠道适配器：`stripe.go`, `alipay.go`, `wxpay.go`, `airwallex.go`, `easypay.go`。`factory.go` 根据配置构造适配器。

适配器负责签名、请求和响应规范化，不负责订单状态机。新增渠道必须同时提供回调验签和错误映射测试。
