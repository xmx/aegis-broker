package restapi

import (
	"github.com/gorilla/websocket"
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-common/library/httpkit"
)

func NewEcho() *Echo {
	return &Echo{
		wsu: httpkit.NewWebsocketUpgrader(),
	}
}

type Echo struct {
	wsu *websocket.Upgrader
}

func (ech *Echo) RegisterRoute(r *ship.RouteGroupBuilder) error {
	r.Route("/echo/chat").GET(ech.chat)

	return nil
}

func (ech *Echo) chat(c *ship.Context) error {
	w, r := c.Response(), c.Request()
	cli, err := ech.wsu.Upgrade(w, r, nil)
	if err != nil {
		c.Errorf("websocket 升级失败", "error", err)
		return nil
	}
	defer cli.Close()

	c.Infof("websocket 升级成功")

	for {
		mt, msg, exx := cli.ReadMessage()
		if exx != nil {
			c.Errorf("读取消息错误", "error", exx)
			break
		}
		c.Infof("[broker receive message] >>> ", "message", string(msg))
		msg = append(msg, []byte(" 响应测试")...)
		if err = cli.WriteMessage(mt, msg); err != nil {
			break
		}
	}
	c.Warnf("websocket disconnect")

	return nil
}
