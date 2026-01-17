package config

// StorageConfig 存储配置（HTTP/FTP 共享）
type StorageConfig struct {
	StorageSiteNo string `mapstructure:"storage_site_no" yaml:"storage_site_no"` // 标识当前部署/站点
	RootPath      string `mapstructure:"root_path" yaml:"root_path"`             // 根存储路径
	TempPath      string `mapstructure:"temp_path" yaml:"temp_path"`             // 临时文件路径
}

// GetStorageSiteNo 获取存储站点标识，有默认值
func (s *StorageConfig) GetStorageSiteNo() string {
	if s == nil || s.StorageSiteNo == "" {
		return "main"
	}
	return s.StorageSiteNo
}

// GetRootPath 获取根存储路径，有默认值
// 这是 HTTP 和 FTP 共享的基础存储路径
// FTP 上传会在此基础上加 /ftp 子目录
func (s *StorageConfig) GetRootPath() string {
	if s == nil || s.RootPath == "" {
		return "./uploads"
	}
	return s.RootPath
}

// GetTempPath 获取临时文件路径，有默认值
func (s *StorageConfig) GetTempPath() string {
	if s == nil || s.TempPath == "" {
		return "../tmp/ftp"
	}
	return s.TempPath
}

var StorageConfigInstance = new(StorageConfig)

