package seed

import (
	"regexp"
	"strconv"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/helper/mysql"
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

func (sm *XtrabackupSeed) isAvailable() bool {
	err := cmd.CommandRun("xtrabackup --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}

func (sm *XtrabackupSeed) getSupportedEngines() []mysql.Engine {
	output, _ := cmd.CommandCombinedOutput("xtrabackup --version", sm.ExecWithSudo)
	xtrabackupVersionString := string(output)
	re := regexp.MustCompile(`version\s*(\d+)`)
	res := re.FindStringSubmatch(xtrabackupVersionString)[1]
	xtrabackupVersion, _ := strconv.Atoi(res)
	if xtrabackupVersion < 8 {
		return []mysql.Engine{mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED}
	}
	return []mysql.Engine{mysql.ROCKSDB, mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED}
}
