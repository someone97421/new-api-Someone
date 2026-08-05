package controller

import (
	"fmt"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/relay"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	"github.com/gin-gonic/gin"
)

func RelayImageTaskFetch(c *gin.Context) {
	info := relaycommon.GenRelayInfoImage(c, nil)
	info.InitChannelMeta(c)
	if apiErr := relay.ImageTaskFetch(c, info); apiErr != nil {
		logger.LogError(c, fmt.Sprintf("image task query error: %s", common.LocalLogPreview(apiErr.Error())))
		apiErr.SetMessage(common.MessageWithRequestId(apiErr.Error(), c.GetString(common.RequestIdKey)))
		c.JSON(apiErr.StatusCode, gin.H{"error": apiErr.ToOpenAIError()})
	}
}
