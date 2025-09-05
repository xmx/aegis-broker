package config

import (
	"context"
	"encoding/json"
	"os"
)

type Dial struct {
	ID        string   `json:"id"        validate:"required,lte=100"`
	Secret    string   `json:"secret"    validate:"required,lte=1000"`
	Addresses []string `json:"addresses" validate:"lte=100"`
}

// Loader 配置加载器。
type Loader interface {
	Load(ctx context.Context) (*Dial, error)
}

type JSON string

func (j JSON) Load(context.Context) (*Dial, error) {
	f, err := os.Open(string(j))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	c := new(Dial)
	if err = json.NewDecoder(f).Decode(c); err != nil {
		return nil, err
	}

	return c, nil
}
