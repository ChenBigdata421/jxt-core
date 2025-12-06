package config

// DuplicateCheckConfig 去重检查配置（HTTP/FTP 共享）
type DuplicateCheckConfig struct {
	Enabled  bool                         `mapstructure:"enabled" yaml:"enabled"`   // 是否启用去重检查
	Strategy string                       `mapstructure:"strategy" yaml:"strategy"` // 策略: filename, content, hybrid
	Filename *DuplicateFilenameConfig     `mapstructure:"filename" yaml:"filename"` // 文件名匹配配置
	Content  *DuplicateContentConfig      `mapstructure:"content" yaml:"content"`   // 内容匹配配置
}

// DuplicateFilenameConfig 文件名去重配置
type DuplicateFilenameConfig struct {
	CaseSensitive bool   `mapstructure:"case_sensitive" yaml:"case_sensitive"` // 是否大小写敏感
	MatchMode     string `mapstructure:"match_mode" yaml:"match_mode"`         // 匹配模式: prefix, exact, regex
}

// DuplicateContentConfig 内容去重配置
type DuplicateContentConfig struct {
	Algorithm string `mapstructure:"algorithm" yaml:"algorithm"` // 哈希算法: sha1, sha256, md5
}

// IsEnabled 检查是否启用去重检查
func (d *DuplicateCheckConfig) IsEnabled() bool {
	return d != nil && d.Enabled
}

// GetStrategy 获取去重策略，默认为 filename
func (d *DuplicateCheckConfig) GetStrategy() string {
	if d == nil || d.Strategy == "" {
		return "filename"
	}
	return d.Strategy
}

// GetFilenameConfig 获取文件名配置，带默认值
func (d *DuplicateCheckConfig) GetFilenameConfig() *DuplicateFilenameConfig {
	if d == nil || d.Filename == nil {
		return &DuplicateFilenameConfig{
			CaseSensitive: false,
			MatchMode:     "prefix",
		}
	}
	return d.Filename
}

// GetContentConfig 获取内容配置，带默认值
func (d *DuplicateCheckConfig) GetContentConfig() *DuplicateContentConfig {
	if d == nil || d.Content == nil {
		return &DuplicateContentConfig{
			Algorithm: "sha1",
		}
	}
	return d.Content
}

var DuplicateCheckConfigInstance = new(DuplicateCheckConfig)

