package seed

import (
	"context"
	"fmt"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	log "github.com/sirupsen/logrus"
)

type LVMSeed struct {
	*Base
	*MethodOpts
	Config *LVMConfig
	Logger *log.Entry
}

type LVMConfig struct {
	Enabled                            bool   `toml:"enabled"`
	CreateSnapshotCommand              string `toml:"create-snapshot-command"`
	AvailableLocalSnapshotHostsCommand string `toml:"available-local-snapshot-hosts-command"`
	AvailableSnapshotHostsCommand      string `toml:"available-snapshot-hosts-command"`
	SnapshotVolumesFilter              string `toml:"snapshot-volumes-filter"`
	SnapshotMountPoint                 string `toml:"snapshot-mount-point"`
}

func (sm *LVMSeed) Prepare(ctx context.Context, side Side) error {
	sm.Logger.Info("This is LVM prepare")
	return nil
}

func (sm *LVMSeed) Backup(ctx context.Context) error {
	sm.Logger.Info("This is LVM backup")
	return nil
}

func (sm *LVMSeed) Restore(ctx context.Context) error {
	sm.Logger.Info("This is LVM restore")
	return nil
}

func (sm *LVMSeed) GetMetadata(ctx context.Context) (*BackupMetadata, error) {
	sm.Logger.Info("This is LVM metadata")
	return &BackupMetadata{}, nil
}

func (sm *LVMSeed) Cleanup(ctx context.Context, side Side) error {
	sm.Logger.Info("This is LVM cleanup")
	return nil
}

func (sm *LVMSeed) IsAvailable() bool {
	err := cmd.CommandRun(fmt.Sprintf("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent,time --sort -time %s", sm.Config.SnapshotVolumesFilter), sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}
