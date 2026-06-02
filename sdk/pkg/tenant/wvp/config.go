package wvp

// TenantWvpConfig represents WVP platform configuration enriched with tenant metadata.
type TenantWvpConfig struct {
	TenantID int
	Code     string
	Name     string
	ApiUrl   string
	Realm    string
}
