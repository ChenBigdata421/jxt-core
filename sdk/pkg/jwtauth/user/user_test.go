package user

import (
	"net/http/httptest"
	"testing"

	jwt "github.com/ChenBigdata421/jxt-core/sdk/pkg/jwtauth"
	"github.com/gin-gonic/gin"
)

func ginContextWithClaims(claims jwt.MapClaims) *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Set(jwt.JwtPayloadKey, claims)
	return c
}

func TestGetPoliceName(t *testing.T) {
	c := ginContextWithClaims(jwt.MapClaims{
		"policename": "张三",
		"nice":       "zhangsan", // login account — must NOT be returned by GetPoliceName
	})
	if got := GetPoliceName(c); got != "张三" {
		t.Fatalf("GetPoliceName want 张三, got %q", got)
	}
}

func TestGetPoliceNameEmptyWhenAbsent(t *testing.T) {
	c := ginContextWithClaims(jwt.MapClaims{})
	if got := GetPoliceName(c); got != "" {
		t.Fatalf("absent claim → \"\", got %q", got)
	}
}
