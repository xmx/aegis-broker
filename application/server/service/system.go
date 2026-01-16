package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/xmx/aegis-broker/application/server/response"
	"github.com/xmx/aegis-broker/config"
	"github.com/xmx/aegis-common/banner"
	"github.com/xmx/aegis-control/datalayer/model"
	"github.com/xmx/aegis-control/datalayer/repository"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func NewSystem(repo repository.All, hide *config.Config, boot model.BrokerConfig, log *slog.Logger) *System {
	return &System{
		repo: repo,
		hide: hide,
		boot: boot,
		log:  log,
	}
}

type System struct {
	repo repository.All
	hide *config.Config
	boot model.BrokerConfig
	log  *slog.Logger
}

func (syt *System) Config() *response.SystemConfig {
	return &response.SystemConfig{
		Hide: syt.hide,
		Boot: syt.boot,
	}
}

func (syt *System) Upgrade(ctx context.Context) (bool, error) {
	info := banner.SelfInfo()
	num := model.ParseSemver(info.Semver)

	attrs := []any{"current", info}
	syt.log.Info("检查升级", attrs)
	filter := bson.M{"goos": info.Goos, "goarch": info.Goarch, "version": bson.M{"$gt": num}}
	repo := syt.repo.BrokerRelease()
	latest, err := repo.FindOne(ctx, filter)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			syt.log.Info("没有找到更新的版本", attrs...)
			return false, nil
		}
		attrs = append(attrs, "error", err)
		syt.log.Warn("检查新版本出错", attrs...)
		return false, nil
	}
	attrs = append(attrs, "latest", latest)
	syt.log.Info("找到了新的版本", attrs)

	settingRepo := syt.repo.Setting()
	setting, err := settingRepo.Get(ctx)
	if err != nil {
		attrs = append(attrs, "error", err)
		syt.log.Warn("缺少全局配置（setting）", attrs...)
		return false, nil
	}
	addresses := setting.Exposes.Addresses()
	if len(addresses) == 0 {
		syt.log.Warn("全局配置缺少接入点（setting.exposes）", attrs...)
		return false, nil
	}

	return true, nil
}

func (syt *System) Exit(after time.Duration) error {
	// FIXME 检查服务文件是否存在，如果不存在就不能退出，会导致服务无法启动。
	//  此方式并不靠谱，测试而已。
	if _, err := os.Stat("/etc/systemd/system/aegis-server.service"); err != nil {
		return err
	}

	// code 是什么不重要，较真的话可以选择一个有意义的 code。
	const code = 75 // https://man.openbsd.org/sysexits.3#EX_TEMPFAIL
	time.AfterFunc(after, func() { os.Exit(code) })

	return nil
}

func (syt *System) name(ctx context.Context, release *model.BrokerRelease) error {
	binaryName := release.Filename
	attrs := []any{"binary_name", binaryName}

	repo := syt.repo.BrokerRelease()
	stm, err := repo.OpenFile(ctx, release.FileID)
	if err != nil {
		attrs = append(attrs, "error", err)
		syt.log.Warn("打开文件出错", attrs...)
		return err
	}
	defer stm.Close()

	binary, err := os.OpenFile(binaryName, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		attrs = append(attrs, "error", err)
		syt.log.Warn("创建新文件出错", attrs...)
		return err
	}
	defer binary.Close()

	offset, err1 := io.Copy(binary, stm)
	if err1 != nil {
		attrs = append(attrs, "error", err1)
		syt.log.Warn("文件写入本地错误", attrs...)
		return err
	}

	// TODO 隐写数据
	_ = offset

	const symlinkName = "aegis-broker"
	if err = os.Remove(symlinkName); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			attrs = append(attrs, "error", err)
			syt.log.Warn("删除旧的符号链接错误", attrs...)
			return err
		}
	}
	if err = os.Symlink(binaryName, symlinkName); err != nil {
		attrs = append(attrs, "error", err)
		syt.log.Error("创建新符号链接错误", attrs...)
		return err
	}

	if err = syt.Exit(time.Second); err != nil {
		syt.log.Error("创建新符号链接错误", attrs...)
	}

	return nil
}
