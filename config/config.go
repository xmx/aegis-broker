package config

type Config struct {
	Secret    string   `json:"secret"    validate:"required,lte=1000"`
	Protocols []string `json:"protocols" validate:"omitempty,lte=2,unique,dive,oneof=tcp udp"`
	Addresses []string `json:"addresses" validate:"lte=100"`
}
