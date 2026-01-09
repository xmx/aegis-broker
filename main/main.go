package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/xmx/aegis-broker/launch"
	"github.com/xmx/aegis-common/banner"
)

func main() {
	set := flag.NewFlagSet(os.Args[0], flag.ExitOnError)
	cfg := set.String("c", "", "配置文件")
	ver := set.Bool("v", false, "打印版本")
	_ = set.Parse(os.Args[1:])
	if _, _ = banner.ANSI(os.Stdout); *ver {
		return
	}

	for _, str := range []string{"resources/.crash.txt", ".crash.txt"} {
		if f, _ := os.Create(str); f != nil {
			_ = debug.SetCrashOutput(f, debug.CrashOptions{})
			_ = f.Close()
			break
		}
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	err := launch.Run(ctx, *cfg)
	cause := context.Cause(ctx)

	slog.Warn("服务停止运行", "error", err, "cause", cause)
}
