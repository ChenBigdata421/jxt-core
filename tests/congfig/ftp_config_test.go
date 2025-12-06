package congfig_test

import (
	"testing"

	"github.com/ChenBigdata421/jxt-core/sdk/config"
	"github.com/stretchr/testify/assert"
)

func TestFTPConfig_GetListenAddr(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.FTPConfig
		expected string
	}{
		{"nil config", nil, ":21"},
		{"empty listen addr", &config.FTPConfig{ListenAddr: ""}, ":21"},
		{"custom listen addr", &config.FTPConfig{ListenAddr: ":2121"}, ":2121"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cfg.GetListenAddr()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFTPConfig_GetPublicHost(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.FTPConfig
		expected string
	}{
		{"nil config", nil, "127.0.0.1"},
		{"empty public host", &config.FTPConfig{PublicHost: ""}, "127.0.0.1"},
		{"custom public host", &config.FTPConfig{PublicHost: "192.168.1.100"}, "192.168.1.100"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cfg.GetPublicHost()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFTPConfig_GetIdleTimeout(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.FTPConfig
		expected int
	}{
		{"nil config", nil, 60},
		{"zero timeout", &config.FTPConfig{IdleTimeout: 0}, 60},
		{"negative timeout", &config.FTPConfig{IdleTimeout: -1}, 60},
		{"custom timeout", &config.FTPConfig{IdleTimeout: 120}, 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cfg.GetIdleTimeout()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFTPConfig_GetConnectionTimeout(t *testing.T) {
	tests := []struct {
		name     string
		cfg      *config.FTPConfig
		expected int
	}{
		{"nil config", nil, 30},
		{"zero timeout", &config.FTPConfig{ConnectionTimeout: 0}, 30},
		{"custom timeout", &config.FTPConfig{ConnectionTimeout: 60}, 60},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.cfg.GetConnectionTimeout()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFTPConfig_GetPassivePortRange(t *testing.T) {
	t.Run("nil config returns defaults", func(t *testing.T) {
		var cfg *config.FTPConfig
		assert.Equal(t, 8091, cfg.GetPassivePortStart())
		assert.Equal(t, 8100, cfg.GetPassivePortEnd())
	})

	t.Run("zero values return defaults", func(t *testing.T) {
		cfg := &config.FTPConfig{}
		assert.Equal(t, 8091, cfg.GetPassivePortStart())
		assert.Equal(t, 8100, cfg.GetPassivePortEnd())
	})

	t.Run("custom values", func(t *testing.T) {
		cfg := &config.FTPConfig{
			PassivePortRange: config.PortRange{
				Start: 9000,
				End:   9100,
			},
		}
		assert.Equal(t, 9000, cfg.GetPassivePortStart())
		assert.Equal(t, 9100, cfg.GetPassivePortEnd())
	})
}

func TestFTPConfig_TLS(t *testing.T) {
	t.Run("nil config TLS disabled", func(t *testing.T) {
		var cfg *config.FTPConfig
		assert.False(t, cfg.IsTLSEnabled())
		assert.Empty(t, cfg.GetTLSCertFile())
		assert.Empty(t, cfg.GetTLSKeyFile())
	})

	t.Run("TLS enabled with files", func(t *testing.T) {
		cfg := &config.FTPConfig{
			TLS: config.FTPTLSConfig{
				Enabled:  true,
				CertFile: "/path/to/cert.pem",
				KeyFile:  "/path/to/key.pem",
			},
		}
		assert.True(t, cfg.IsTLSEnabled())
		assert.Equal(t, "/path/to/cert.pem", cfg.GetTLSCertFile())
		assert.Equal(t, "/path/to/key.pem", cfg.GetTLSKeyFile())
	})
}

func TestFTPConfig_Users(t *testing.T) {
	t.Run("nil config returns nil users", func(t *testing.T) {
		var cfg *config.FTPConfig
		assert.Nil(t, cfg.GetUsers())
		assert.Nil(t, cfg.FindUser("admin"))
	})

	t.Run("find user", func(t *testing.T) {
		cfg := &config.FTPConfig{
			Users: []config.FTPUserConfig{
				{Username: "admin", Password: "admin123", HomeDirectory: "/home/admin", WritePermission: true},
				{Username: "guest", Password: "guest123", HomeDirectory: "/home/guest", WritePermission: false},
			},
		}

		users := cfg.GetUsers()
		assert.Len(t, users, 2)

		admin := cfg.FindUser("admin")
		assert.NotNil(t, admin)
		assert.Equal(t, "admin", admin.Username)
		assert.True(t, admin.WritePermission)

		guest := cfg.FindUser("guest")
		assert.NotNil(t, guest)
		assert.False(t, guest.WritePermission)

		unknown := cfg.FindUser("unknown")
		assert.Nil(t, unknown)
	})
}

