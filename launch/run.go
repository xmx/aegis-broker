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
	"github.com/xmx/aegis-broker/channel/gateway"
	"github.com/xmx/aegis-broker/channel/tundial"
	"github.com/xmx/aegis-broker/channel/tunnel"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-common/library/cronv3"
	"github.com/xmx/aegis-common/library/httpx"
	"github.com/xmx/aegis-common/logger"
	"github.com/xmx/aegis-common/shipx"
	"github.com/xmx/aegis-common/transport"
	"github.com/xmx/aegis-common/validation"
	"github.com/xmx/aegis-control/contract/linkhub"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/mongodb"
	"github.com/xmx/aegis-control/quick"
	"golang.org/x/net/quic"
)

func Exec(ctx context.Context, cld config.Loader) error {
	consoleOut := logger.NewTint(os.Stdout)
	logHandler := logger.NewHandler(consoleOut)
	log := slog.New(logHandler)

	cfg, err := cld.Load(ctx)
	if err != nil {
		log.Error("配置加载错误", slog.Any("error", err))
		return err
	}

	valid := validation.New()
	_ = valid.RegisterCustomValidations(validation.Customs())
	if err = valid.Validate(cfg); err != nil {
		log.Error("配置验证错误", slog.Any("error", err))
		return err
	}

	crond := cronv3.New(ctx, log, cron.WithSeconds())
	crond.Start()
	defer crond.Stop()

	log.Info("向中心端建立连接中...")
	srvHandler := httpx.NewAtomicHandler(nil)
	dial := tunnel.DialConfig{
		ID:        cfg.ID,
		Secret:    cfg.Secret,
		Addresses: cfg.Addresses,
		Handler:   srvHandler,
		DialConfig: transport.DialConfig{
			Parent:    ctx,
			Protocols: cfg.Protocols,
		},
		Timeout: 5 * time.Second,
		Logger:  log,
	}
	cli, err := dial.Open()
	if err != nil {
		return err
	}

	log.Info("向中心端请求初始配置")
	initCfg, err := cli.InitialConfig(ctx)
	if err != nil {
		log.Error("向中心端请求初始配置错误", slog.Any("error", err))
		return err
	}
	mongoURI := initCfg.MongoURI
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
	thisBrk, err := brokerBiz.Get(ctx, cfg.ID)
	if err != nil {
		return err
	}
	agentSvc := expservice.NewAgent(thisBrk, repoAll, log)
	_ = agentSvc.Reset(ctx)

	hub := linkhub.NewHub(4096)
	multiDial := tundial.NewDialer(cli, hub)
	multiTrip := &http.Transport{DialContext: multiDial.DialContext}
	agtHandler := httpx.NewAtomicHandler(nil)
	agtGate := gateway.New(thisBrk, repoAll, hub, valid, agtHandler, log)
	exposeAPIs := []shipx.RouteRegister{
		exprestapi.NewTunnel(agtGate),
	}
	serverAPIs := []shipx.RouteRegister{
		srvrestapi.NewReverse(multiTrip),
		srvrestapi.NewCertificate(certificateBiz, log),
		srvrestapi.NewSystem(cfg),
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
	agtHandler.Store(agtSH)
	{
		apiRGB := agtSH.Group("/api")
		if err = shipx.RegisterRoutes(apiRGB, agentAPIs); err != nil {
			return err
		}
	}

	_, _ = crond.AddTask(crontab.NewNetwork(thisBrk.ID, repoAll, log))

	brkCfg := thisBrk.Config
	listenAddr := brkCfg.Listen
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
	quicSrv := &quick.Server{
		Addr:    listenAddr,
		Handler: agtGate,
		QUICConfig: &quic.Config{
			TLSConfig: tlsCfg,
		},
	}

	errs := make(chan error)
	go listenHTTP(errs, httpSrv, log)
	go listenQUIC(ctx, errs, quicSrv, log)
	select {
	case err = <-errs:
	case <-ctx.Done():
	}
	_ = httpSrv.Close()
	_ = quicSrv.Close()
	_ = cli.Close()
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

func listenQUIC(ctx context.Context, errs chan<- error, srv *quick.Server, log *slog.Logger) {
	addr := srv.Addr
	if addr == "" {
		addr = ":443"
	}

	endpoint, err := quic.Listen("udp", addr, srv.QUICConfig)
	if err != nil {
		errs <- err
		return
	}
	defer endpoint.Close(context.Background())

	laddr := endpoint.LocalAddr()
	log.Warn("quic 服务监听成功", "listen", laddr)

	errs <- srv.Serve(ctx, endpoint)
}
