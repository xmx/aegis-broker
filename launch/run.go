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
	agtrestapi "github.com/xmx/aegis-broker/applet/agent/restapi"
	agtservice "github.com/xmx/aegis-broker/applet/agent/service"
	"github.com/xmx/aegis-broker/applet/crontab"
	exprestapi "github.com/xmx/aegis-broker/applet/expose/restapi"
	expservice "github.com/xmx/aegis-broker/applet/expose/service"
	srvrestapi "github.com/xmx/aegis-broker/applet/server/restapi"
	"github.com/xmx/aegis-broker/business"
	"github.com/xmx/aegis-broker/channel/clientd"
	"github.com/xmx/aegis-broker/channel/serverd"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/library/httpkit"
	"github.com/xmx/aegis-common/library/validation"
	"github.com/xmx/aegis-common/logger"
	"github.com/xmx/aegis-common/profile"
	"github.com/xmx/aegis-common/shipx"
	"github.com/xmx/aegis-common/tunnel/tundial"
	"github.com/xmx/aegis-common/tunnel/tunutil"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/linkhub"
	"github.com/xmx/aegis-control/mongodb"
	"github.com/xmx/aegis-control/quick"
)

func Run(ctx context.Context, cfg string) error {
	// 2<<22 = 8388608 (8 MiB)
	opt := profile.NewOption().Limit(2 << 22).ModuleName("aegis/broker/config")
	crd := profile.NewFile[config.Config](cfg, opt)

	return Exec(ctx, crd)
}

func Exec(ctx context.Context, crd profile.Reader[config.Config]) error {
	consoleOut := logger.NewTint(os.Stdout)
	logHandler := logger.NewHandler(consoleOut)
	log := slog.New(logHandler)

	hideCfg, err := crd.Read(ctx)
	if err != nil {
		log.Error("配置加载错误", slog.Any("error", err))
		return err
	}

	valid := validation.New()
	_ = valid.RegisterCustomValidations(validation.Customs())
	if err = valid.Validate(hideCfg); err != nil {
		log.Error("配置验证错误", slog.Any("error", err))
		return err
	}

	crond := cronv3.New(ctx, log, cron.WithSeconds())
	crond.Start()
	defer crond.Stop()

	log.Info("向中心端建立连接中...")
	srvHandler := httpkit.NewAtomicHandler(nil)
	dialCfg := tundial.Config{
		Protocols:  hideCfg.Protocols,
		Addresses:  hideCfg.Addresses,
		PerTimeout: 10 * time.Second,
		Parent:     ctx,
	}
	clientdOpt := clientd.NewOption().Handler(srvHandler).Logger(log)
	mux, initialCfg, err := clientd.Open(dialCfg, hideCfg.Secret, clientdOpt)
	if err != nil {
		return err
	}

	log.Info("向中心端请求初始配置")
	mongoURI := initialCfg.Config.URI
	log.Debug("开始连接数据库", slog.Any("mongo_uri", mongoURI))
	db, err := mongodb.Open(mongoURI)
	if err != nil {
		log.Error("数据库连接错误", slog.Any("error", err))
		return err
	}
	log.Info("数据库连接成功")

	repoAll := repository.NewAll(db)
	brokerBiz := business.NewBroker(repoAll, log)
	certificateBiz := business.NewCertificate(repoAll, log)

	// 查询自己的配置
	curBroker, err := brokerBiz.FindBySecret(ctx, hideCfg.Secret)
	if err != nil {
		return err
	}

	brokerID := curBroker.ID
	bootCfg := curBroker.Config
	agentSvc := expservice.NewAgent(curBroker, repoAll, log)
	_ = agentSvc.Reset(ctx)

	hub := linkhub.NewHub(4096)

	systemDialer := tunutil.DefaultDialer()
	muxDialer := tunutil.NewMuxDialer(mux)
	serverDialer := tunutil.NewHostMatchDialer(tunutil.ServerHost, muxDialer)
	agentDialer := linkhub.NewSuffixDialer(hub, tunutil.AgentHostSuffix)
	multiDialer := tunutil.NewMatchDialer(systemDialer, agentDialer, serverDialer)

	tunnelInnerHandler := httpkit.NewAtomicHandler(nil)
	serverdOpt := serverd.NewOption().Handler(tunnelInnerHandler).Validator(valid).Logger(log).Huber(hub)
	tunnelAccept := serverd.New(curBroker, repoAll, serverdOpt)
	exposeAPIs := []shipx.RouteRegister{
		exprestapi.NewTunnel(tunnelAccept),
	}

	serverAPIs := []shipx.RouteRegister{
		srvrestapi.NewReverse(multiDialer),
		srvrestapi.NewCertificate(certificateBiz, log),
		srvrestapi.NewEcho(),
		srvrestapi.NewSystem(hideCfg, bootCfg),
		shipx.NewHealth(),
		shipx.NewPprof(),
	}
	var agentAPIs []shipx.RouteRegister
	{
		systemSvc := agtservice.NewSystem(repoAll, log)
		agentAPIs = append(agentAPIs,
			agtrestapi.NewSystem(systemSvc),
		)
	}

	shipLog := logger.NewShip(logHandler, 6)
	srvSH := ship.Default()
	srvSH.NotFound = shipx.NotFound
	srvSH.HandleError = shipx.HandleErrorWithHost("broker")
	srvSH.Validator = valid
	srvSH.Logger = shipLog
	srvHandler.Store(srvSH)

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

	agtSH := ship.Default()
	agtSH.NotFound = shipx.NotFound
	agtSH.HandleError = shipx.HandleError
	agtSH.Validator = valid
	agtSH.Logger = shipLog
	tunnelInnerHandler.Store(agtSH)
	{
		apiRGB := agtSH.Group("/api")
		if err = shipx.RegisterRoutes(apiRGB, agentAPIs); err != nil {
			return err
		}
	}

	cronTasks := []cronv3.Tasker{
		crontab.NewNetwork(brokerID, repoAll),
		crontab.NewTransmit(brokerID, mux, hub, repoAll),
	}
	for _, task := range cronTasks {
		_, _ = crond.AddTask(task)
	}

	listenAddr := bootCfg.Server.Addr
	if listenAddr == "" {
		listenAddr = ":443"
	}

	tlsCfg := &tls.Config{
		GetCertificate:     certificateBiz.GetCertificate,
		NextProtos:         []string{"http/1.1", "h2", "aegis"},
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true,
	}
	httpSrv := &http.Server{
		Addr:      listenAddr,
		Handler:   exposeSH,
		TLSConfig: tlsCfg,
	}
	quicSrv := &quick.QUICGo{
		Addr:      listenAddr,
		Handler:   tunnelAccept,
		TLSConfig: tlsCfg,
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
		_ = agentSvc.Reset(cctx)
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
