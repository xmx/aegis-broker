package restapi

import (
	"fmt"

	"github.com/gorilla/websocket"
	"github.com/xgfone/ship/v5"
	"github.com/xmx/aegis-common/library/wsocket"
)

func NewEcho() *Echo {
	return &Echo{
		wsu: wsocket.NewUpgrade(),
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
		return nil
	}
	defer cli.Close()

	conn := cli.NetConn()
	fmt.Println(conn)

	for {
		mt, msg, err := cli.ReadMessage()
		if err != nil {
			break
		}
		c.Infof("[broker message] >>> ", "message", string(msg))
		msg = append(msg, []byte(" echo 测试\n")...)
		if err = cli.WriteMessage(mt, msg); err != nil {
			break
		}
	}

	return nil
}
