package user

import (
	"fmt"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg"
	jwt "github.com/ChenBigdata421/jxt-core/sdk/pkg/jwtauth"
	"github.com/gin-gonic/gin"
)

func ExtractClaims(c *gin.Context) jwt.MapClaims {
	claims, exists := c.Get(jwt.JwtPayloadKey)
	if !exists {
		return make(jwt.MapClaims)
	}

	return claims.(jwt.MapClaims)
}

func Get(c *gin.Context, key string) interface{} {
	data := ExtractClaims(c)
	if data[key] != nil {
		return data[key]
	}

	fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " Get 缺少 " + key)

	return nil
}

// GetUserId 获取一个int的userId
func GetUserId(c *gin.Context) int {
	data := ExtractClaims(c)
	identity, err := data.Identity()
	if err != nil {
		fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " GetUserId 缺少 identity error: " + err.Error())
		return 0
	}

	return int(identity)
}

// GetUserIdInt64 获得int64的userId
func GetUserIdInt64(c *gin.Context) int64 {
	data := ExtractClaims(c)
	identity, err := data.Identity()
	if err != nil {
		fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " GetUserId 缺少 identity error: " + err.Error())
		return 0
	}

	return identity
}

func GetUserIdStr(c *gin.Context) string {
	data := ExtractClaims(c)

	return data.String("identity")
}

func GetUserName(c *gin.Context) string {
	return ExtractClaims(c).String("nice")
}

func GetRoleName(c *gin.Context) string {
	return ExtractClaims(c).String("rolekey")
}

func GetRoleId(c *gin.Context) int {
	roleId, err := ExtractClaims(c).Int("roleid")
	if err != nil {
		fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " GetRoleId 缺少 roleid error: " + err.Error())
		return 0
	}

	return roleId
}

// GetOrgId 获取组织ID
func GetOrgId(c *gin.Context) int {
	orgId, err := ExtractClaims(c).Int("orgid")
	if err != nil {
		fmt.Println(pkg.GetCurrentTimeStr() + " [WARING] " + c.Request.Method + " " + c.Request.URL.Path + " GetOrgId 缺少 orgid error: " + err.Error())
		return 0
	}

	return orgId
}

// GetOrgName 获取组织名称
func GetOrgName(c *gin.Context) string {
	return ExtractClaims(c).String("orgname")
}
