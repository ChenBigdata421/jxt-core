package config

var TenantsConfig = new(Tenants)

type Tenants struct {
	Enabled bool `mapstructure:"enabled"` //是否启用租户配置
	//默认配置的作用：
	//1.每个租户只需配置与默认值不同的部分，大幅减少配置文件的体积和复杂度;
	//2.为所有租户提供一个基准配置，确保关键参数的一致性。
	//3.当需要更改通用配置时，只需修改默认值而非每个租户的配置。
	//4.当租户特定配置缺失或无效时，系统可以平滑降级到默认配置。
	Resolver Resolver       `mapstructure:"resolver"`
	Defaults DefaultConfig  `mapstructure:"defaults"`
	List     []TenantConfig `mapstructure:"list"` //租户列表
}

// Resolver 租户识别配置
type Resolver struct {
	Type       string `mapstructure:"type"`       // 租户识别方式: host, path, header, query
	HeaderName string `mapstructure:"headerName"` // 当type为header时使用的header名称
	QueryParam string `mapstructure:"queryParam"` // 当type为query时使用的查询参数名
	PathIndex  int    `mapstructure:"pathIndex"`  // 当type为path时使用的路径索引
}

type DefaultConfig struct {
	Database DatabaseDefaults `mapstructure:"database"`
}

type DatabaseDefaults struct {
	Driver       string `mapstructure:"driver"`
	MaxOpenConns int    `mapstructure:"maxOpenConns"`
	MaxIdleConns int    `mapstructure:"maxIdleConns"`
}

type TenantConfig struct {
	ID       string   `mapstructure:"id"`       //租户ID，唯一标识一个租户
	Name     string   `mapstructure:"name"`     //租户名称，便于识别和管理
	Active   bool     `mapstructure:"active"`   //租户是否处于活动状态
	Hosts    []string `mapstructure:"hosts"`    //允许一个租户关联多个主机名（域名/子域名）提供了显著的灵活性和业务价值
	Database Database `mapstructure:"database"` //租户数据库配置
}

// 配置举例
/*
# 多租户系统配置
tenants:
  # 是否启用多租户功能
  enabled: true

  # 租户识别配置
  resolver:
    type: "host"  # 可选: host, path, header, query
    headerName: "X-Tenant-ID"  # 当type为header时使用
    queryParam: "tenant"  # 当type为query时使用
    pathIndex: 0  # 当type为path时使用

  # 默认配置 - 适用于所有租户的基础设置
  defaults:
    database:
      driver: "mysql"
      maxOpenConns: 10
      maxIdleConns: 5

  # 租户列表 - 每个租户的特定配置
  list:
    # 第一个租户配置
    - id: "tenant_main"
      name: "Primary Organization"
      active: true
      hosts:
        - "example.com"
        - "www.example.com"
      database:
        driver: "mysql"  # 可选，覆盖默认值
        maxOpenConns: 20  # 可选，覆盖默认值
        maxIdleConns: 10  # 可选，覆盖默认值
        source: "user:pass@tcp(primary-db.example.com:3306)/main_db"

    # 第二个租户配置
    - id: "tenant_subsidiary"
      name: "Subsidiary Company"
      active: true
      hosts:
        - "subsidiary.com"
        - "sub.example.com"
        - "legacy-subsidiary.net"  # 支持旧域名
      database:
        # 继承默认数据库驱动和连接设置
        source: "user:pass@tcp(sub-db.example.com:3306)/subsidiary_db"

    # 第三个租户 - 使用不同数据库驱动
    - id: "tenant_international"
      name: "International Division"
      active: true
      hosts:
        - "intl.example.com"
        - "example.co.uk"
        - "example.de"
      database:
        driver: "postgres"  # 覆盖默认MySQL设置
        maxOpenConns: 15
        source: "postgres://user:pass@intl-db.example.com:5432/intl_db"

    # 第四个租户 - 暂时停用
    - id: "tenant_inactive"
      name: "Inactive Project"
      active: false  # 租户暂时禁用
      hosts:
        - "inactive.example.com"
      database:
        source: "user:pass@tcp(db.example.com:3306)/inactive_db"

    # 第五个租户 - 多品牌企业示例
    - id: "tenant_multi_brand"
      name: "Multi-Brand Enterprise"
      active: true
      hosts:
        - "brand1.com"
        - "brand2.com"
        - "brand3.com"
        - "parent-company.com"
      database:
        source: "user:pass@tcp(db.example.com:3306)/multi_brand_db"
        maxOpenConns: 30  # 由于多个域名，增加连接数
*/
