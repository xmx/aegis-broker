package restapi

import (
	"net/http"
	"os"
	"time"

	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-broker/application/server/response"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-control/datalayer/model"
)

func NewSystem(hide *config.Config, boot model.BrokerConfig) *System {
	return &System{
		hide: hide,
		boot: boot,
	}
}

type System struct {
	hide *config.Config
	boot model.BrokerConfig
}

func (syt *System) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/system/config").GET(syt.config)
	r.Route("/system/ping").GET(syt.ping)
	r.Route("/system/exit").GET(syt.exit)
	return nil
}

func (syt *System) config(c *ship.Context) error {
	ret := &response.SystemConfig{
		Hide: syt.hide,
		Boot: syt.boot,
	}

	return c.JSON(http.StatusOK, ret)
}

func (*System) ping(c *ship.Context) error {
	return c.NoContent(http.StatusNoContent)
}

func (syt *System) exit(*ship.Context) error {
	// FIXME 检查服务文件是否存在，如果不存在就不能退出，会导致服务无法启动。
	//  此方式并不靠谱，测试而已。
	if _, err := os.Stat("/etc/systemd/system/aegis-server.service"); err != nil {
		return err
	}

	time.AfterFunc(3*time.Second, func() { os.Exit(0) })

	return nil
}
