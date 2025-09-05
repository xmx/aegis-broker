package launch

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"
	"os"

	"github.com/xgfone/ship/v5"
	exprestapi "github.com/xmx/aegis-broker/applet/expose/restapi"
	srvrestapi "github.com/xmx/aegis-broker/applet/server/restapi"
	"github.com/xmx/aegis-broker/business"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-broker/tunnel/bclient"
	"github.com/xmx/aegis-common/library/httpx"
	"github.com/xmx/aegis-common/logger"
	"github.com/xmx/aegis-common/shipx"
	"github.com/xmx/aegis-common/validation"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/mongodb"
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
	if err = valid.Validate(cfg); err != nil {
		log.Error("配置验证错误", slog.Any("error", err))
		return err
	}

	log.Info("向中心端建立连接中...")
	srvHandle := httpx.NewAtomicHandler(nil)
	cli, err := bclient.Open(ctx, cfg, srvHandle, log)
	if err != nil {
		return err
	}

	log.Info("向中心端请求初始配置")
	dbCfg, err := cli.Config(ctx)
	if err != nil {
		log.Error("向中心端请求初始配置错误", slog.Any("error", err))
		return err
	}
	mongoURI := dbCfg.URI
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

	exposeAPIs := []shipx.RouteRegister{
		exprestapi.NewHealth(),
	}
	serverAPIs := []shipx.RouteRegister{
		srvrestapi.NewCertificate(certificateBiz, log),
		srvrestapi.NewHealth(),
	}

	shipLog := logger.NewShip(logHandler, 6)
	srvSH := ship.Default()
	srvSH.NotFound = shipx.NotFound
	srvSH.HandleError = shipx.HandleError
	srvSH.Validator = valid
	srvSH.Logger = shipLog
	srvHandle.Store(srvSH)

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

	brkCfg := thisBrk.Config
	listenAddr := brkCfg.Listen
	if listenAddr == "" {
		listenAddr = ":443"
	}

	tlsCfg := &tls.Config{
		GetCertificate:     certificateBiz.GetCertificate,
		NextProtos:         []string{"http/1.1", "h2", "aegis"},
		InsecureSkipVerify: true,
	}
	srv := &http.Server{
		Addr:      listenAddr,
		Handler:   exposeSH,
		TLSConfig: tlsCfg,
	}
	errs := make(chan error)
	go listenAndServe(errs, srv, log)
	select {
	case err = <-errs:
	case <-ctx.Done():
	}
	_ = srv.Close()

	if err != nil {
		log.Error("程序运行错误", slog.Any("error", err))
	} else {
		log.Warn("程序结束运行")
	}

	return nil
}

func listenAndServe(errs chan<- error, srv *http.Server, log *slog.Logger) {
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
