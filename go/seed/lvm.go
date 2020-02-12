package seed

import (
	"fmt"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/helper/mysql"
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

func (sm *LVMSeed) Prepare(side Side) {
	sm.Logger.Info("This is LVM prepare")
}

func (sm *LVMSeed) Backup(seedHost string, mysqlPort int) {
	sm.Logger.Info("This is LVM backup")
}

func (sm *LVMSeed) Restore() {
	sm.Logger.Info("This is LVM restore")
}

func (sm *LVMSeed) GetMetadata() (*SeedMetadata, error) {
	sm.Logger.Info("This is LVM metadata")
	return &SeedMetadata{}, nil
}

func (sm *LVMSeed) Cleanup(side Side) {
	sm.Logger.Info("This is LVM cleanup")
}

func (sm *LVMSeed) isAvailable() bool {
	err := cmd.CommandRun(fmt.Sprintf("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent,time --sort -time %s", sm.Config.SnapshotVolumesFilter), sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}

func (sm *LVMSeed) getSupportedEngines() []mysql.Engine {
	return []mysql.Engine{mysql.ROCKSDB, mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED, mysql.TokuDB}
}

func (sm *LVMSeed) backupToDatadir() bool {
	return true
}
