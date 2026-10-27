package controller

import (
	"io"
	"net/http"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	hosttypes "github.com/QuantumNous/new-api/types"
	"github.com/gin-gonic/gin"
)

func relayChannelAttempt(c *gin.Context, info *relaycommon.RelayInfo, format types.RelayFormat) *types.NewAPIError {
	if format != types.RelayFormatOpenAIRealtime {
		settings, _ := common.GetContextKeyType[dto.ChannelSettings](c, constant.ContextKeyChannelSetting)
		restore, err := common.ApplyChannelBodyTextReplacement(c, settings.BodyTextReplacement)
		if err != nil {
			status := http.StatusBadRequest
			if common.IsRequestBodyTooLargeError(err) {
				status = http.StatusRequestEntityTooLarge
			}
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, status, types.ErrOptionWithSkipRetry())
		}
		defer restore()
		request, err := helper.GetAndValidateRequest(c, format)
		if err != nil {
			return types.NewErrorWithStatusCode(err, types.ErrorCodeInvalidRequest, http.StatusBadRequest, types.ErrOptionWithSkipRetry())
		}
		fresh, err := relaycommon.GenRelayInfo(c, format, request, info.ClientWs)
		if err != nil {
			return types.NewError(err, types.ErrorCodeGenRelayInfoFailed, types.ErrOptionWithSkipRetry())
		}
		info.Request = request
		info.IsStream = fresh.IsStream
		info.ReasoningEffort = fresh.ReasoningEffort
		info.ReasoningConversion = nil
		info.ResponsesUsageInfo = fresh.ResponsesUsageInfo
		info.ClaudeConvertInfo = fresh.ClaudeConvertInfo
		info.RerankerInfo = fresh.RerankerInfo
		info.RequestHeaders = fresh.RequestHeaders
		info.ThinkingContentInfo = fresh.ThinkingContentInfo
		info.TieredBillingSnapshot = nil
		info.BillingRequestInput = nil
		info.BillingImageCount = nil
		info.ImageRequestCount = 0
		info.ImageQuotaBeforeGroup = 0
		info.PriceData = hosttypes.PriceData{}
	}
	info.InitChannelMeta(c)
	if err := relay.PrepareRequestBilling(c, info); err != nil {
		return err
	}
	if info.Billing != nil {
		info.PriceData.FreeModel = false
	}
	storage, err := common.GetBodyStorage(c)
	if err != nil {
		status := http.StatusBadRequest
		if common.IsRequestBodyTooLargeError(err) {
			status = http.StatusRequestEntityTooLarge
		}
		return types.NewErrorWithStatusCode(err, types.ErrorCodeReadRequestBodyFailed, status, types.ErrOptionWithSkipRetry())
	}
	c.Request.Body = io.NopCloser(storage)
	switch format {
	case types.RelayFormatOpenAIRealtime:
		return relay.WssHelper(c, info)
	case types.RelayFormatClaude:
		return relay.ClaudeHelper(c, info)
	case types.RelayFormatGemini:
		return geminiRelayHandler(c, info)
	default:
		return relayHandler(c, info)
	}
}
