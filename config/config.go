package config

type Config struct {
	Secret    string   `json:"secret,omitzero"    validate:"required,lte=1000"`
	Protocols []string `json:"protocols,omitzero" validate:"omitempty,lte=4,unique,dive,oneof=tcp udp udp-quic-std udp-quic-go"`
	Addresses []string `json:"addresses,omitzero" validate:"lte=100"`
	Offset    int64    `json:"offset,omitzero"`
}
