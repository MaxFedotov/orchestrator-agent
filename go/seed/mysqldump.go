package seed

import (
	"context"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	log "github.com/sirupsen/logrus"
)

type MysqldumpSeed struct {
	*Base
	*MethodOpts
	Config *MysqldumpConfig
	Logger *log.Entry
}

type MysqldumpConfig struct {
	Enabled  bool `toml:"enabled"`
	Compress bool `toml:"compress"`
}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
)

func (sm *MysqldumpSeed) Prepare(ctx context.Context, side Side) error {
	sm.Logger.Info("This is mysqldump prepare")
	return nil
}

func (sm *MysqldumpSeed) Backup(ctx context.Context) error {
	sm.Logger.Info("This is mysqldump backup")
	return nil
}

func (sm *MysqldumpSeed) Restore(ctx context.Context) error {
	sm.Logger.Info("This is mysqldump restore")
	return nil
}

func (sm *MysqldumpSeed) GetMetadata(ctx context.Context) (*BackupMetadata, error) {
	sm.Logger.Info("This is mysqldump metadata")
	return &BackupMetadata{}, nil
}

func (sm *MysqldumpSeed) Cleanup(ctx context.Context, side Side) error {
	sm.Logger.Info("This is mysqldump cleanup")
	return nil
}

func (sm *MysqldumpSeed) IsAvailable() bool {
	err := cmd.CommandRun("mysqldump --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}
