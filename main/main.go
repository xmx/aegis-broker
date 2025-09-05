package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"

	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-broker/launch"
)

func main() {
	args := os.Args
	name := filepath.Base(args[0])
	set := flag.NewFlagSet(name, flag.ExitOnError)
	ver := set.Bool("v", false, "打印版本")
	_ = set.Parse(args[1:])
	if *ver {
		return
	}

	for _, str := range []string{"resources/.crash.txt", ".crash.txt"} {
		if f, _ := os.Create(str); f != nil {
			_ = debug.SetCrashOutput(f, debug.CrashOptions{})
			_ = f.Close()
			break
		}
	}

	signals := []os.Signal{syscall.SIGTERM, syscall.SIGHUP, syscall.SIGINT, syscall.SIGQUIT}
	ctx, cancel := signal.NotifyContext(context.Background(), signals...)
	defer cancel()

	testCfg := &config.Boot{
		ID:      "68b1572248fd2282fa1756aa",
		Secret:  "b02185a09070296c6ada4dc1e40cd9aa0b0bd5a9141b2639b4500cce724228342302fc6c3c5b084904d5ff5a68302f1986d0",
		Address: "aegis.eastmoney.dev",
	}

	if err := launch.Exec(ctx, testCfg); err != nil {
		slog.Error("服务运行错误", slog.Any("error", err))
	} else {
		slog.Info("服务停止运行")
	}
}
