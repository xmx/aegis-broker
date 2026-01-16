package config

type Config struct {
	Secret    string   `json:"secret,omitzero"    validate:"required,lte=1000"`
	Semver    string   `json:"semver,omitzero"    validate:"omitempty,semver"`
	Protocols []string `json:"protocols,omitzero" validate:"omitempty,lte=4,unique,dive,oneof=quic quic-go smux yamux"`
	Addresses []string `json:"addresses,omitzero" validate:"lte=100"`
	Offset    int64    `json:"offset,omitzero"`
}
