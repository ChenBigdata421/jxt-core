package config

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDefaultTenantConfig_ServiceDatabases(t *testing.T) {
	Convey("测试服务级数据库配置", t, func() {

		Convey("HasServiceDatabases - 判断是否有服务级配置", func() {
			config := &DefaultTenantConfig{}

			Convey("无配置时应返回 false", func() {
				So(config.HasServiceDatabases(), ShouldBeFalse)
			})

			Convey("有空 map 时应返回 false", func() {
				config.ServiceDatabases = make(map[string]TenantDatabaseDetailConfig)
				So(config.HasServiceDatabases(), ShouldBeFalse)
			})

			Convey("有配置时应返回 true", func() {
				config.ServiceDatabases = map[string]TenantDatabaseDetailConfig{
					"evidence-command": {Driver: "mysql"},
				}
				So(config.HasServiceDatabases(), ShouldBeTrue)
			})
		})

		Convey("GetServiceDatabases - 获取服务级配置映射", func() {
			config := &DefaultTenantConfig{
				ServiceDatabases: map[string]TenantDatabaseDetailConfig{
					"evidence-command": {
						Driver:   "mysql",
						Host:     "mysql-command",
						Port:     3306,
						Database: "tenant_command",
					},
					"evidence-query": {
						Driver:   "postgres",
						Host:     "postgres-query",
						Port:     5432,
						Database: "tenant_query",
					},
				},
			}

			result := config.GetServiceDatabases()
			So(len(result), ShouldEqual, 2)
			So(result["evidence-command"].Driver, ShouldEqual, "mysql")
			So(result["evidence-query"].Driver, ShouldEqual, "postgres")
		})

		Convey("GetServiceDatabase - 获取指定服务配置", func() {
			config := &DefaultTenantConfig{
				ServiceDatabases: map[string]TenantDatabaseDetailConfig{
					"evidence-command": {
						Driver:   "mysql",
						Host:     "mysql-command",
						Database: "tenant_command",
					},
				},
			}

			Convey("存在的服务应返回配置", func() {
				result := config.GetServiceDatabase("evidence-command")
				So(result, ShouldNotBeNil)
				So(result.Driver, ShouldEqual, "mysql")
				So(result.Database, ShouldEqual, "tenant_command")
			})

			Convey("不存在的服务应返回 nil", func() {
				result := config.GetServiceDatabase("non-existent")
				So(result, ShouldBeNil)
			})

			Convey("nil config 应返回 nil", func() {
				var nilConfig *DefaultTenantConfig = nil
				result := nilConfig.GetServiceDatabase("evidence-command")
				So(result, ShouldBeNil)
			})
		})

		Convey("ServiceDatabases 配置应正常工作", func() {
			config := &DefaultTenantConfig{
				ServiceDatabases: map[string]TenantDatabaseDetailConfig{
					"evidence-command": {Driver: "mysql"},
					"evidence-query":   {Driver: "postgres"},
				},
			}

			So(config.HasServiceDatabases(), ShouldBeTrue)
			So(len(config.ServiceDatabases), ShouldEqual, 2)
		})
	})
}
