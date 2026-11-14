package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/handler/dto"
	"github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	"github.com/gin-gonic/gin"
)

func adminRedeemCodeForRequest(c *gin.Context, code *service.RedeemCode) *dto.AdminRedeemCode {
	result := dto.RedeemCodeFromServiceAdmin(code)
	if result != nil && !middleware.CanReadRedeemSecrets(c) {
		result.Code = "********"
	}
	return result
}

func redactPaymentVoucher(c *gin.Context, order *AdminPaymentOrderResult) *AdminPaymentOrderResult {
	if order != nil && !middleware.CanReadRedeemSecrets(c) {
		order.RechargeCode = ""
	}
	return order
}
