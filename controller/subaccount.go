package controller

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
)

func ListSubaccounts(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		writeSubaccountError(c, model.ErrSubaccountForbidden)
		return
	}
	children, err := model.ListSubaccounts(identity.UserID)
	if err != nil {
		writeSubaccountError(c, err)
		return
	}
	common.ApiSuccess(c, children)
}

func CreateSubaccount(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		writeSubaccountError(c, model.ErrSubaccountForbidden)
		return
	}
	params := map[string]any{"success": false}
	defer recordUserSecurityAudit(c, identity.UserID, "subaccount.create", params)
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		writeSubaccountError(c, model.ErrInvalidSubaccountInput)
		return
	}
	child, err := model.CreateSubaccount(identity, request.Username, request.Password)
	if err != nil {
		writeSubaccountError(c, err)
		return
	}
	params["success"] = true
	params["target_user_id"] = child.Id
	common.ApiSuccess(c, child)
}

func DisableSubaccount(c *gin.Context) {
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		writeSubaccountError(c, model.ErrSubaccountForbidden)
		return
	}
	params := map[string]any{"success": false}
	defer recordUserSecurityAudit(c, identity.UserID, "subaccount.disable", params)
	childID, err := strconv.Atoi(c.Param("id"))
	if err != nil || childID <= 0 {
		writeSubaccountError(c, model.ErrInvalidSubaccountInput)
		return
	}
	params["target_user_id"] = childID
	if err := model.DisableSubaccount(identity, childID); err != nil {
		writeSubaccountError(c, err)
		return
	}
	params["success"] = true
	common.ApiSuccess(c, nil)
}

func writeSubaccountError(c *gin.Context, err error) {
	status, code, message := http.StatusInternalServerError, "SUBACCOUNT_OPERATION_FAILED", "Please try again later."
	switch {
	case errors.Is(err, model.ErrSubaccountForbidden):
		status, code, message = http.StatusForbidden, "SECURITY_ACTION_FORBIDDEN", "Unauthorized, insufficient privileges"
	case errors.Is(err, model.ErrSubaccountReauth):
		status, code, message = http.StatusForbidden, "RECENT_LOGIN_REQUIRED", "Please sign in again."
	case errors.Is(err, model.ErrInvalidSubaccountInput), errors.Is(err, common.ErrAccountPasswordLength), errors.Is(err, common.ErrPasswordLegacyLimit):
		status, code, message = http.StatusBadRequest, "INVALID_PARAMETERS", "Invalid parameters"
	}
	c.Set("security_error_code", code)
	common.ApiErrorStatus(c, status, common.NewMessage(message), gin.H{"code": code})
}
