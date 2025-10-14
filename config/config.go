package config

type Config struct {
	Secret    string   `json:"secret"    validate:"required,lte=1000"`
	Protocols []string `json:"protocols" validate:"omitempty,lte=4,unique,dive,oneof=tcp udp udp-quic-std udp-quic-go"`
	Addresses []string `json:"addresses" validate:"lte=100"`
}
