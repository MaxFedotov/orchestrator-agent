package seed

import (
	"context"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	log "github.com/sirupsen/logrus"
)

type XtrabackupSeed struct {
	*Base
	*MethodOpts
	Config *XtrabackupConfig
	Logger *log.Entry
}

type XtrabackupConfig struct {
	Enabled         bool `toml:"enabled"`
	ParallelThreads int  `toml:"parallel-threads"`
	Compress        bool `toml:"compress"`
}

func (sm *XtrabackupSeed) Prepare(ctx context.Context, side Side) error {
	sm.Logger.Info("This is xtrabackup prepare")
	return nil
}

func (sm *XtrabackupSeed) Backup(ctx context.Context) error {
	sm.Logger.Info("This is xtrabackup backup")
	return nil
}

func (sm *XtrabackupSeed) Restore(ctx context.Context) error {
	sm.Logger.Info("This is xtrabackup restore")
	return nil
}

func (sm *XtrabackupSeed) GetMetadata(ctx context.Context) (*BackupMetadata, error) {
	sm.Logger.Info("This is xtrabackup metadata")
	return &BackupMetadata{}, nil
}

func (sm *XtrabackupSeed) Cleanup(ctx context.Context, side Side) error {
	sm.Logger.Info("This is xtrabackup cleanup")
	return nil
}

func (sm *XtrabackupSeed) IsAvailable() bool {
	err := cmd.CommandRun("xtrabackup --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}
