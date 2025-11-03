package serverd

type authRequest struct {
	MachineID  string   `json:"machine_id"          validate:"required,gte=10,lte=100"`
	Goos       string   `json:"goos"                validate:"required,oneof=darwin dragonfly illumos ios js wasip1 linux android solaris freebsd nacl netbsd openbsd plan9 windows aix"`
	Goarch     string   `json:"goarch"              validate:"required,oneof=386 amd64 arm arm64 loong64 mips mipsle mips64 mips64le ppc64 ppc64le riscv64 s390x sparc64 wasm"`
	PID        int      `json:"pid,omitzero"`
	Args       []string `json:"args,omitzero"`
	Hostname   string   `json:"hostname,omitzero"`
	Workdir    string   `json:"workdir,omitzero"`
	Executable string   `json:"executable,omitzero"`
	Username   string   `json:"username,omitzero"`
}

type authResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitzero"`
}
