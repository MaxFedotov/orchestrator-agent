package seed

import (
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

func (sm *XtrabackupSeed) Prepare(side Side) {
	// start socat to listen on target, clean datadir, stop mysql
	sm.Logger.Info("This is xtrabackup prepare")
}

func (sm *XtrabackupSeed) Backup(seedHost string, mysqlPort int) {
	sm.Logger.Info("This is xtrabackup backup")
}

func (sm *XtrabackupSeed) Restore() {
	sm.Logger.Info("This is xtrabackup restore")
}

func (sm *XtrabackupSeed) GetMetadata() (*BackupMetadata, error) {
	sm.Logger.Info("This is xtrabackup metadata")
	return &BackupMetadata{}, nil
}

func (sm *XtrabackupSeed) Cleanup(side Side) {
	sm.Logger.Info("This is xtrabackup cleanup")
}

func (sm *XtrabackupSeed) IsAvailable() bool {
	err := cmd.CommandRun("xtrabackup --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}
