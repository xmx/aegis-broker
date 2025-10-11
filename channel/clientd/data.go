package clientd

type authResponse struct {
	Code    int    `json:"code"`    // 状态码
	Message string `json:"message"` // 错误信息
}

func (ar authResponse) successful() bool {
	return ar.Code >= 200 && ar.Code < 300
}

type authRequest struct {
	MachineID  string   `json:"machine_id,omitzero"`
	Goos       string   `json:"goos,omitzero"`
	Goarch     string   `json:"goarch,omitzero"`
	PID        int      `json:"pid,omitzero"`
	Args       []string `json:"args,omitzero"`
	Hostname   string   `json:"hostname,omitzero"`
	Workdir    string   `json:"workdir,omitzero"`
	Executable string   `json:"executable,omitzero"`
	Username   string   `json:"username,omitzero"`
	UID        string   `json:"uid,omitzero"`
}
