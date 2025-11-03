package clientd

import (
	"fmt"
	"net/http"
)

type authRequest struct {
	Secret     string   `json:"secret"`
	Inet       string   `json:"inet"`
	Goos       string   `json:"goos"`
	Goarch     string   `json:"goarch"`
	PID        int      `json:"pid,omitzero"`
	Args       []string `json:"args,omitzero"`
	Hostname   string   `json:"hostname,omitzero"`
	Workdir    string   `json:"workdir,omitzero"`
	Executable string   `json:"executable,omitzero"`
	Username   string   `json:"username,omitzero"`
}

type authResponse struct {
	Code    int        `json:"code"`    // 状态码
	Message string     `json:"message"` // 错误信息
	Config  AuthConfig `json:"config"`
}

type AuthConfig struct {
	URI string `json:"uri"` // mongo 连接
}

func (ar authResponse) checkError() error {
	code := ar.Code
	if code >= http.StatusOK && code < http.StatusMultipleChoices {
		return nil
	}

	return fmt.Errorf("agent 认证失败 %d: %s", ar.Code, ar.Message)
}
