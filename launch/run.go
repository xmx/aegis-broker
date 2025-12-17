package launch

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xgfone/ship/v5"
	agtrestapi "github.com/xmx/aegis-broker/application/agent/restapi"
	agtservice "github.com/xmx/aegis-broker/application/agent/service"
	"github.com/xmx/aegis-broker/application/business"
	"github.com/xmx/aegis-broker/application/crontab"
	exprestapi "github.com/xmx/aegis-broker/application/expose/restapi"
	expservice "github.com/xmx/aegis-broker/application/expose/service"
	srvrestapi "github.com/xmx/aegis-broker/application/server/restapi"
	srvservice "github.com/xmx/aegis-broker/application/server/service"
	"github.com/xmx/aegis-broker/channel/clientd"
	"github.com/xmx/aegis-broker/channel/serverd"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/library/validation"
	"github.com/xmx/aegis-common/logger"
	"github.com/xmx/aegis-common/profile"
	"github.com/xmx/aegis-common/shipx"
	"github.com/xmx/aegis-common/stegano"
	"github.com/xmx/aegis-common/tunnel/tunconst"
	"github.com/xmx/aegis-common/tunnel/tundial"
	"github.com/xmx/aegis-common/tunnel/tunopen"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/linkhub"
	"github.com/xmx/aegis-control/mongodb"
	"github.com/xmx/aegis-control/quick"
	"github.com/xmx/aegis-control/tlscert"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"gopkg.in/natefinch/lumberjack.v2"
)

func Run(ctx context.Context, cfg string) error {
	var cfr profile.Reader[config.Config]
	if cfg != "" {
		cfr = profile.File[config.Config](cfg)
	} else {
		exe := os.Args[0]
		cfr = stegano.File[config.Config](exe)
	}

	return Exec(ctx, cfr)
}

func Exec(ctx context.Context, crd profile.Reader[config.Config]) error {
	consoleOut := logger.NewTint(os.Stdout, nil)
	logh := logger.NewMultiHandler(consoleOut)
	log := slog.New(logh)

	hideCfg, err := crd.Read()
	if err != nil {
		log.Error("配置加载错误", slog.Any("error", err))
		return err
	}

	valid := validation.New()
	_ = valid.RegisterCustomValidations(validation.All())
	if err = valid.Validate(hideCfg); err != nil {
		log.Error("配置验证错误", slog.Any("error", err))
		return err
	}

	crond := cronv3.New(ctx, log, cron.WithSeconds())
	crond.Start()
	defer crond.Stop()

	log.Info("向中心端建立连接中...")

	shipLog := logger.NewShip(logh)
	srvSH := ship.Default()
	srvSH.NotFound = shipx.NotFound
	srvSH.HandleError = shipx.HandleError
	srvSH.Validator = valid
	srvSH.Logger = shipLog

	dialCfg := tunopen.Config{
		Protocols:  hideCfg.Protocols,
		Addresses:  hideCfg.Addresses,
		PerTimeout: 10 * time.Second,
		Parent:     ctx,
	}
	clientdOpt := clientd.NewOption().Handler(srvSH).Logger(log)
	mux, initialCfg, err := clientd.Open(dialCfg, hideCfg.Secret, clientdOpt)
	if err != nil {
		return err
	}

	log.Info("向中心端请求初始配置")
	mongoURI := initialCfg.URI
	log.Debug("开始连接数据库", slog.Any("mongo_uri", mongoURI))
	mongoLogOpt := options.Logger().
		SetSink(logger.NewSink(logh)).
		SetComponentLevel(options.LogComponentCommand, options.LogLevelDebug)
	mongoOpt := options.Client().SetLoggerOptions(mongoLogOpt)
	db, err := mongodb.Open(mongoURI, mongoOpt)
	if err != nil {
		log.Error("数据库连接错误", slog.Any("error", err))
		return err
	}
	log.Info("数据库连接成功")

	// 查询自己的配置
	repoAll := repository.NewAll(db, log)
	curBroker, err := repoAll.Broker().GetBySecret(ctx, hideCfg.Secret)
	if err != nil {
		return err
	}
	bcfg := curBroker.Config
	lc := bcfg.Logger

	logLevel := new(slog.LevelVar)
	_ = logLevel.UnmarshalText([]byte(lc.Level))
	logOpt := &slog.HandlerOptions{AddSource: true, Level: logLevel}
	logh.Replace()
	if lc.Console {
		tint := logger.NewTint(os.Stdout, logOpt)
		logh.Attach(tint)
	}
	if fname := lc.Filename; fname != "" {
		lumber := &lumberjack.Logger{
			Filename:   fname,
			MaxSize:    lc.MaxSize,
			MaxAge:     lc.MaxAge,
			MaxBackups: lc.MaxBackups,
			LocalTime:  lc.LocalTime,
			Compress:   lc.Compress,
		}
		defer lumber.Close()

		lh := slog.NewJSONHandler(lumber, logOpt)
		logh.Attach(lh)
	}

	loadCert := repoAll.Certificate().Enables
	certPool := tlscert.NewCertPool(loadCert, false, log)

	brokerID := curBroker.ID
	agentSvc := expservice.NewAgent(repoAll, log)
	victoriaMetricsSvc := business.NewVictoriaMetrics(repoAll, curBroker, log)
	_ = agentSvc.Reset(ctx, curBroker.ID)

	hub := linkhub.NewHub(4096)
	netDialer := &net.Dialer{Timeout: 30 * time.Second}
	tunDialers := []tundial.ContextDialer{
		tundial.NewMatchHostDialer(tunconst.ServerHost, mux),
		linkhub.NewSuffixDialer(tunconst.AgentHostSuffix, hub),
	}
	dualDialer := tundial.NewFirstMatchDialer(tunDialers, netDialer)
	_ = &http.Client{Transport: &http.Transport{DialContext: dualDialer.DialContext}}

	agtSH := ship.Default()
	agtSH.NotFound = shipx.NotFound
	agtSH.HandleError = shipx.HandleError
	agtSH.Validator = valid
	agtSH.Logger = shipLog

	serverdOpt := serverd.NewOption().
		Handler(agtSH).
		Valid(valid.Validate).
		Logger(log).
		Huber(hub)
	tunnelAccept := serverd.New(curBroker, repoAll, serverdOpt)
	exposeAPIs := []shipx.RouteRegister{
		exprestapi.NewTunnel(tunnelAccept),
	}

	srvSystemSvc := srvservice.NewSystem(repoAll, hideCfg, bcfg, log)
	serverAPIs := []shipx.RouteRegister{
		srvrestapi.NewReverse(dualDialer),
		srvrestapi.NewEcho(),
		srvrestapi.NewSystem(srvSystemSvc),
		shipx.NewHealth(),
		shipx.NewPprof(),
	}
	var agentAPIs []shipx.RouteRegister
	{
		systemSvc := agtservice.NewSystem(repoAll, log)
		agentAPIs = append(agentAPIs,
			agtrestapi.NewSystem(systemSvc),
			agtrestapi.NewVictoriaMetrics(victoriaMetricsSvc),
		)
	}

	// server RPC 路由注册。
	{
		apiRGB := srvSH.Group("/api")
		if err = shipx.RegisterRoutes(apiRGB, serverAPIs); err != nil {
			return err
		}
	}

	exposeSH := ship.Default()
	exposeSH.NotFound = shipx.NotFound
	exposeSH.HandleError = shipx.HandleError
	exposeSH.Validator = valid
	exposeSH.Logger = shipLog
	{
		apiRGB := exposeSH.Group("/api")
		if err = shipx.RegisterRoutes(apiRGB, exposeAPIs); err != nil {
			return err
		}
	}

	// agent RPC 路由注册。
	{
		apiRGB := agtSH.Group("/api")
		if err = shipx.RegisterRoutes(apiRGB, agentAPIs); err != nil {
			return err
		}
	}

	cronTasks := []cronv3.Tasker{
		crontab.NewMetrics(curBroker, victoriaMetricsSvc.PushConfig),
		crontab.NewNetwork(brokerID, repoAll),
		crontab.NewTransmit(brokerID, mux, hub, repoAll),
		crontab.NewTransmitMetrics(curBroker, mux, hub, victoriaMetricsSvc.PushConfig),
	}
	for _, task := range cronTasks {
		_, _ = crond.AddTask(task)
	}

	listenAddr := bcfg.Server.Addr
	if listenAddr == "" {
		listenAddr = ":443"
	}

	httpTLS := &tls.Config{GetCertificate: certPool.Match, MinVersion: tls.VersionTLS13} // TLSv1.3 绕过阿里云未备案域名拦截。
	quicTLS := &tls.Config{GetCertificate: certPool.Match, MinVersion: tls.VersionTLS13, NextProtos: []string{"aegis"}}
	httpSrv := &http.Server{
		Addr:      listenAddr,
		Handler:   exposeSH,
		TLSConfig: httpTLS,
	}
	quicSrv := &quick.QUICGo{
		Addr:      listenAddr,
		Handler:   tunnelAccept,
		TLSConfig: quicTLS,
	}

	errs := make(chan error, 2)
	go listenHTTP(errs, httpSrv, log)
	go listenQUIC(ctx, errs, quicSrv)
	select {
	case err = <-errs:
	case <-ctx.Done():
	}
	_ = httpSrv.Close()
	_ = quicSrv.Close()
	_ = mux.Close()
	{
		cctx, ccancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = agentSvc.Reset(cctx, curBroker.ID)
		ccancel()
	}

	if err != nil {
		log.Error("程序运行错误", slog.Any("error", err))
	} else {
		log.Warn("程序结束运行")
	}

	return nil
}

func listenHTTP(errs chan<- error, srv *http.Server, log *slog.Logger) {
	lc := new(net.ListenConfig)
	lc.SetMultipathTCP(true)
	ln, err := lc.Listen(context.Background(), "tcp", srv.Addr)
	if err != nil {
		errs <- err
		return
	}
	laddr := ln.Addr().String()
	log.Warn("http 服务监听成功", "listen", laddr)

	errs <- srv.ServeTLS(ln, "", "")
}

func listenQUIC(ctx context.Context, errs chan<- error, srv quick.Server) {
	errs <- srv.ListenAndServe(ctx)
}
