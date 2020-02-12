package seed

import (
	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/helper/mysql"
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

func (sm *MydumperSeed) Prepare(side Side) {
	sm.Logger.Info("This is mydumper prepare")
}

func (sm *MydumperSeed) Backup(seedHost string, mysqlPort int) {
	sm.Logger.Info("This is mydumper backup")
}

func (sm *MydumperSeed) Restore() {
	sm.Logger.Info("This is mydumper restore")
}

func (sm *MydumperSeed) GetMetadata() (*SeedMetadata, error) {
	sm.Logger.Info("This is mydumper metadata")
	return &SeedMetadata{}, nil
}

func (sm *MydumperSeed) Cleanup(side Side) {
	sm.Logger.Info("This is mydumper cleanup")
}

func (sm *MydumperSeed) isAvailable() bool {
	err := cmd.CommandRun("mydumper --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}

func (sm *MydumperSeed) getSupportedEngines() []mysql.Engine {
	return []mysql.Engine{mysql.ROCKSDB, mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED, mysql.TokuDB}
}

func (sm *MydumperSeed) backupToDatadir() bool {
	return false
}
