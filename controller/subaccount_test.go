package controller

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubaccountManagementAuthorization(t *testing.T) {
	db := setupManageUserTestDB(t)
	require.NoError(t, db.AutoMigrate(&model.Token{}))
	// Initialize the dialect-specific column quoting used by API key lookups.
	t.Setenv("LOG_SQL_DSN", "")
	logDB := model.LOG_DB
	require.NoError(t, model.InitLogDB())
	model.LOG_DB = logDB
	oldNewUser, oldInviter, oldInvitee := common.QuotaForNewUser, common.QuotaForInviter, common.QuotaForInvitee
	redisServer := miniredis.RunT(t)
	previousRedisEnabled, previousRDB := common.RedisEnabled, common.RDB
	common.RedisEnabled = true
	common.RDB = redis.NewClient(&redis.Options{Addr: redisServer.Addr(), MaxRetries: -1})
	t.Cleanup(func() {
		_ = common.RDB.Close()
		common.RedisEnabled, common.RDB = previousRedisEnabled, previousRDB
	})
	common.QuotaForNewUser, common.QuotaForInviter, common.QuotaForInvitee = 5000, 1000, 2000
	t.Cleanup(func() {
		common.QuotaForNewUser, common.QuotaForInviter, common.QuotaForInvitee = oldNewUser, oldInviter, oldInvitee
	})

	parent := model.User{Username: "subaccount-parent", AffCode: "parent-test", Quota: 1200}
	other := model.User{Username: "subaccount-other", AffCode: "other-test"}
	require.NoError(t, db.Create(&parent).Error)
	require.NoError(t, db.Create(&other).Error)

	login := func(userID int) *service.AuthBundle {
		bundle, err := service.CreateLoginSession(userID, "password", "127.0.0.1", "subaccount-test")
		require.NoError(t, err)
		return bundle
	}
	parentSession, otherSession := login(parent.Id), login(other.Id)
	oldSession := login(parent.Id)
	oldIdentity, err := service.ParseAccessToken(oldSession.AccessToken)
	require.NoError(t, err)
	require.NoError(t, db.Model(&model.UserSession{}).Where("sid = ?", oldIdentity.SessionID).
		Update("created_at", time.Now().Add(-time.Hour).Unix()).Error)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	const path = "/api/user/self/subaccounts"
	r.GET(path, middleware.UserAuth(), ListSubaccounts)
	r.POST(path, middleware.UserAuth(), CreateSubaccount)
	r.POST(path+"/:id/disable", middleware.UserAuth(), DisableSubaccount)
	r.GET("/api/user/self", middleware.UserAuth(), GetSelf)
	accepted := func(c *gin.Context) { c.Status(http.StatusNoContent) }
	r.GET("/relay", middleware.TokenAuth(), accepted)
	r.GET("/relay-usage", middleware.TokenAuthReadOnly(), accepted)
	request := func(method, url, token, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, strings.NewReader(body))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		res := httptest.NewRecorder()
		r.ServeHTTP(res, req)
		return res
	}

	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, path, "", "").Code)
	assert.Contains(t, request(http.MethodGet, path, parentSession.AccessToken, "").Body.String(), `"data":[]`)
	body := `{"username":"child-test","password":"child-password","parent_id":` + strconv.Itoa(other.Id) + `,"role":100,"quota":1000,"inviter_id":` + strconv.Itoa(other.Id) + `}`
	assert.Contains(t, request(http.MethodPost, path, oldSession.AccessToken, body).Body.String(), `"code":"RECENT_LOGIN_REQUIRED"`)
	created := request(http.MethodPost, path, parentSession.AccessToken, body)
	require.Equal(t, http.StatusOK, created.Code, created.Body.String())
	assert.NotContains(t, created.Body.String(), "child-password")
	var child model.User
	require.NoError(t, db.Where("username = ?", "child-test").First(&child).Error)
	assert.Equal(t, parent.Id, child.ParentId)
	assert.Zero(t, child.InviterId)
	assert.Zero(t, child.Quota)
	childCacheKey := "user:" + strconv.Itoa(child.Id)
	assert.Equal(t, strconv.Itoa(parent.Id), redisServer.HGet(childCacheKey, "ParentId"))
	redisServer.HSet(childCacheKey, "ParentId", strconv.Itoa(other.Id))
	cachedChild, err := model.GetUserCache(child.Id)
	require.NoError(t, err)
	assert.Equal(t, parent.Id, cachedChild.ParentId)
	assert.Equal(t, strconv.Itoa(parent.Id), redisServer.HGet(childCacheKey, "ParentId"))
	assert.Equal(t, common.RoleCommonUser, child.Role)
	assert.NotEqual(t, "child-password", child.Password)
	childSession := login(child.Id)
	assert.Contains(t, request(http.MethodGet, "/api/user/self", childSession.AccessToken, "").Body.String(), `"is_subaccount":true`)
	assert.Contains(t, request(http.MethodGet, "/api/user/self", childSession.AccessToken, "").Body.String(), `"quota":1200`)
	assert.Equal(t, http.StatusForbidden, request(http.MethodGet, path, childSession.AccessToken, "").Code)
	assert.Equal(t, http.StatusForbidden, request(http.MethodPost, path, childSession.AccessToken, `{"username":"grandchild","password":"child-password"}`).Code)

	list := request(http.MethodGet, path, parentSession.AccessToken, "")
	require.Equal(t, http.StatusOK, list.Code)
	assert.Contains(t, list.Body.String(), `"username":"child-test"`)
	assert.Contains(t, list.Body.String(), `"request_count":0`)
	assert.Contains(t, list.Body.String(), `"used_quota":0`)
	assert.Contains(t, request(http.MethodGet, path, otherSession.AccessToken, "").Body.String(), `"data":[]`)
	disable := path + "/" + strconv.Itoa(child.Id) + "/disable"
	foreign := request(http.MethodPost, disable, otherSession.AccessToken, "")
	missing := request(http.MethodPost, path+"/999999999/disable", otherSession.AccessToken, "")
	assert.Equal(t, http.StatusForbidden, foreign.Code)
	assert.Equal(t, missing.Body.String(), foreign.Body.String())
	assert.Equal(t, http.StatusForbidden, request(http.MethodPost, disable, oldSession.AccessToken, "").Code)

	key, err := common.GenerateKey()
	require.NoError(t, err)
	token := model.Token{UserId: child.Id, Key: key, Status: common.TokenStatusEnabled, UnlimitedQuota: true, ExpiredTime: -1}
	require.NoError(t, token.Insert())
	for _, route := range []string{"/relay", "/relay-usage"} {
		assert.Equal(t, http.StatusNoContent, request(http.MethodGet, route, "sk-"+key, "").Code)
	}
	assert.Equal(t, http.StatusOK, request(http.MethodPost, disable, parentSession.AccessToken, "").Code)
	assert.Equal(t, http.StatusOK, request(http.MethodPost, disable, parentSession.AccessToken, "").Code)
	assert.Equal(t, http.StatusUnauthorized, request(http.MethodGet, "/api/user/self", childSession.AccessToken, "").Code)
	for _, route := range []string{"/relay", "/relay-usage"} {
		assert.Equal(t, http.StatusForbidden, request(http.MethodGet, route, "sk-"+key, "").Code)
	}
	disabledChild, err := model.GetUserById(child.Id, false)
	require.NoError(t, err)
	assert.Equal(t, common.UserStatusDisabled, disabledChild.Status)
	assert.Equal(t, child.AuthVersion+1, disabledChild.AuthVersion)
}
