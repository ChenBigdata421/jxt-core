// Package jwtauth 已弃用
//
// Deprecated: 请使用 github.com/go-admin-team/go-admin-core/jwtauth 替代。
// 本包将在 v2.0.0 版本中移除。
//
// 迁移指南:
//   旧导入: import "github.com/go-admin-team/go-admin-core/sdk/pkg/jwtauth"
//   新导入: import "github.com/go-admin-team/go-admin-core/jwtauth"
package jwtauth

import (
	newjwtauth "github.com/go-admin-team/go-admin-core/jwtauth"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// Deprecated: 使用 github.com/go-admin-team/go-admin-core/jwtauth.MapClaims 替代
type MapClaims = newjwtauth.MapClaims

// Deprecated: 使用 github.com/go-admin-team/go-admin-core/jwtauth.GinJWTMiddleware 替代
type GinJWTMiddleware = newjwtauth.GinJWTMiddleware

// Deprecated: 使用 github.com/go-admin-team/go-admin-core/jwtauth.New 替代
func New(m *GinJWTMiddleware) (*GinJWTMiddleware, error) {
	return newjwtauth.New(m)
}

// Deprecated: 使用 github.com/go-admin-team/go-admin-core/jwtauth.ExtractClaims 替代
func ExtractClaims(c *gin.Context) MapClaims {
	return newjwtauth.ExtractClaims(c)
}

// Deprecated: 使用 github.com/go-admin-team/go-admin-core/jwtauth.ExtractClaimsFromToken 替代
func ExtractClaimsFromToken(token *jwt.Token) MapClaims {
	return newjwtauth.ExtractClaimsFromToken(token)
}

// Deprecated: 使用 github.com/go-admin-team/go-admin-core/jwtauth.GetToken 替代
func GetToken(c *gin.Context) string {
	return newjwtauth.GetToken(c)
}
