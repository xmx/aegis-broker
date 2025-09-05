package launch

import (
	"context"
	"crypto/tls"
	"log/slog"
	"net"
	"net/http"

	"github.com/xgfone/ship/v5"
	exprestapi "github.com/xmx/aegis-broker/applet/expose/restapi"
	srvrestapi "github.com/xmx/aegis-broker/applet/server/restapi"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-broker/tunnel/bclient"
	"github.com/xmx/aegis-common/library/httpx"
	"github.com/xmx/aegis-common/shipx"
	"github.com/xmx/aegis-common/validation"
	"github.com/xmx/aegis-control/datalayer/repository"
	"github.com/xmx/aegis-control/mongodb"
	expservice "github.com/xmx/aegis-server/applet/expose/service"
)

func Exec(ctx context.Context, boot *config.Boot) error {
	valid := validation.New()
	log := slog.Default()
	dialCfg := bclient.DialConfig{
		ID:        boot.ID,
		Secret:    boot.Secret,
		Addresses: []string{boot.Address},
	}
	srvHandle := httpx.NewAtomicHandler(nil)
	cli, err := bclient.Open(ctx, dialCfg, srvHandle, log)
	if err != nil {
		return err
	}

	dbCfg, err := cli.Config(ctx)
	if err != nil {
		return err
	}
	log.Info("配置", slog.Any("config", dbCfg))
	mongoURI := dbCfg.URI
	db, err := mongodb.Open(mongoURI)
	if err != nil {
		return err
	}
	repoAll := repository.NewAll(db)
	certificateSvc := expservice.NewCertificate(repoAll, log)

	exposeAPIs := []shipx.RouteRegister{
		exprestapi.NewHealth(),
	}
	serverAPIs := []shipx.RouteRegister{
		srvrestapi.NewHealth(),
	}

	srvSH := ship.Default()
	srvSH.NotFound = shipx.NotFound
	srvSH.HandleError = shipx.HandleError
	srvSH.Validator = valid
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

	{
		apiRGB := exposeSH.Group("/api")
		if err = shipx.RegisterRoutes(apiRGB, exposeAPIs); err != nil {
			return err
		}
	}

	tlsCfg := &tls.Config{
		GetCertificate: certificateSvc.GetCertificate,
	}
	srv := &http.Server{
		Addr:      ":9443",
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
