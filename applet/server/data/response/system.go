package response

import (
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-control/datalayer/model"
)

type SystemConfig struct {
	Hide *config.Config     `json:"hide"`
	Boot model.BrokerConfig `json:"boot"`
}
