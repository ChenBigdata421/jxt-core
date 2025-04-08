package restapi

import (
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/logger"

	"github.com/ChenBigdata421/jxt-core/sdk/pkg"
	"github.com/ChenBigdata421/jxt-core/sdk/pkg/response"
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type RestApi struct{}

// GetLogger 获取上下文提供的日志器，对GetRequestLogger做封装，可实现解耦。
func (e *RestApi) GetLogger(c *gin.Context) *zap.Logger {
	return logger.GetRequestLogger(c)
}

// Error 通常错误数据处理
func (e *RestApi) Error(c *gin.Context, code int, err error, msg string) {
	response.Error(c, code, err, msg)
}

// OK 通常成功数据处理
func (e *RestApi) OK(c *gin.Context, data interface{}, msg string) {
	response.OK(c, data, msg)
}

// PageOK 分页数据处理
func (e *RestApi) PageOK(c *gin.Context, result interface{}, count int, pageIndex int, pageSize int, msg string) {
	response.PageOK(c, result, count, pageIndex, pageSize, msg)
}

// Custom 兼容函数
func (e *RestApi) Custom(c *gin.Context, data gin.H) {
	response.Custum(c, data)
}

func (e RestApi) Translate(form, to interface{}) {
	pkg.Translate(form, to)
}
