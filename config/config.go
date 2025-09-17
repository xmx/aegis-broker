package config

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"os"
)

type Config struct {
	Mode      string   `json:"mode"      validate:"omitempty,oneof=auto tcp udp"`
	ID        string   `json:"id"        validate:"required,lte=100"`
	Secret    string   `json:"secret"    validate:"required,lte=1000"`
	Addresses []string `json:"addresses" validate:"lte=100"`
}

// Loader 配置加载器。
type Loader interface {
	Load(ctx context.Context) (*Config, error)
}

type JSON string

func (j JSON) Load(context.Context) (*Config, error) {
	f, err := os.Open(string(j))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := new(Config)
	dec := jsontext.NewDecoder(f)
	if err = json.UnmarshalDecode(dec, c); err != nil {
		return nil, err
	}

	return c, nil
}
