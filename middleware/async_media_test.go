package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAsyncMediaRequestOnlyInterceptsSupportedPostEndpoints(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name   string
		method string
		url    string
		want   bool
	}{
		{name: "image JSON", method: http.MethodPost, url: "/v1/images/generations?async=true", want: true},
		{name: "image multipart", method: http.MethodPost, url: "/v1/images/edits?async=1", want: true},
		{name: "video", method: http.MethodPost, url: "/v1/videos?async=true", want: true},
		{name: "sync remains sync", method: http.MethodPost, url: "/v1/images/generations", want: false},
		{name: "unsupported chat", method: http.MethodPost, url: "/v1/chat/completions?async=true", want: false},
		{name: "status query", method: http.MethodGet, url: "/v1/images/generations/task?async=true", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, test.url, nil)
			context, _ := gin.CreateTestContext(httptest.NewRecorder())
			context.Request = request
			require.NotNil(t, context.Request)
			assert.Equal(t, test.want, test.method == http.MethodPost && isAsyncMediaRequest(context))
		})
	}
}
