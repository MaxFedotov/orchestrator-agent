package seed

import (
	"context"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	log "github.com/sirupsen/logrus"
)

type MydumperSeed struct {
	*Base
	*MethodOpts
	Config *MydumperConfig
	Logger *log.Entry
}

type MydumperConfig struct {
	Enabled         bool `toml:"enabled"`
	ParallelThreads int  `toml:"parallel-threads"`
	RowsChunkSize   int  `toml:"rows-chunk-size"`
	Compress        bool `toml:"compress"`
}

func (sm *MydumperSeed) Prepare(ctx context.Context, side Side) error {
	sm.Logger.Info("This is mydumper prepare")
	return nil
}

func (sm *MydumperSeed) Backup(ctx context.Context) error {
	sm.Logger.Info("This is mydumper backup")
	return nil
}

func (sm *MydumperSeed) Restore(ctx context.Context) error {
	sm.Logger.Info("This is mydumper restore")
	return nil
}

func (sm *MydumperSeed) GetMetadata(ctx context.Context) (*BackupMetadata, error) {
	sm.Logger.Info("This is mydumper metadata")
	return &BackupMetadata{}, nil
}

func (sm *MydumperSeed) Cleanup(ctx context.Context, side Side) error {
	sm.Logger.Info("This is mydumper cleanup")
	return nil
}

func (sm *MydumperSeed) IsAvailable() bool {
	err := cmd.CommandRun("mydumper --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}
