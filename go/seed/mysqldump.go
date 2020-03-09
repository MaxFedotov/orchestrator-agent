package seed

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/helper/mysql"
	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
)

var defaultMysqldumpOpts = map[string]bool{
	"--host":          true,
	"-h":              true,
	"--user":          true,
	"-u":              true,
	"--port":          true,
	"-P":              true,
	"--password":      true,
	"-p":              true,
	"--master-data":   true,
	"--all-databases": true,
	"-A":              true,
}

type MysqldumpSeed struct {
	*Base
	*MethodOpts
	Config         *MysqldumpConfig
	Logger         *log.Entry
	BackupFileName string
}

type MysqldumpConfig struct {
	Enabled                 bool     `toml:"enabled"`
	MysqldumpAdditionalOpts []string `toml:"addtional-opts"`
}

func (sm *MysqldumpSeed) Prepare(side Side) {
	stage := NewSeedStage(Prepare, sm.StatusChan)
	sm.Logger.Info("Starting perpare")
	if side == Target {
		cleanupCmd := fmt.Sprintf("rm -rf %s", path.Join(sm.BackupDir, sm.BackupFileName))
		err := cmd.CommandRunWithFunc(cleanupCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Running prepare", sm.StatusChan)
		})
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
	}
	sm.Logger.Info("Prepare completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *MysqldumpSeed) Backup(seedHost string, mysqlPort int) {
	stage := NewSeedStage(Backup, sm.StatusChan)
	var addtionalOpts []string
	for _, opt := range sm.Config.MysqldumpAdditionalOpts {
		if defaultMysqldumpOpts[strings.Split(opt, "=")[0]] {
			sm.Logger.WithField("MysqldumpOption", opt).Error("Will skip mysqldump option, as it is already used by default")
		} else {
			addtionalOpts = append(addtionalOpts, opt)
		}
	}
	backupCmd := fmt.Sprintf("mysqldump --host=%s --user=%s --password=%s --port=%d --master-data=2 --all-databases %s", seedHost, sm.SeedUser, sm.SeedPassword, mysqlPort, strings.Join(addtionalOpts, " "))
	backupCmd += fmt.Sprintf(" > %s", path.Join(sm.BackupDir, sm.BackupFileName))
	sm.Logger.Info("Starting backup")
	err := cmd.CommandRunWithFunc(backupCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Running backup", sm.StatusChan)
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Backup failed")
		return
	}
	sm.Logger.Info("Backup completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *MysqldumpSeed) Restore() {
	stage := NewSeedStage(Restore, sm.StatusChan)
	sm.Logger.Info("Starting restore")
	if err := mysql.Exec(sm.MySQLClient.Conn, "RESET MASTER;"); err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	restoreCmd := fmt.Sprintf("cat %s | mysql -u%s -p%s --port %d", path.Join(sm.BackupDir, sm.BackupFileName), sm.SeedUser, sm.SeedPassword, sm.MySQLPort)
	err := cmd.CommandRunWithFunc(restoreCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Running restore", sm.StatusChan)
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	sm.Logger.Info("Restore completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *MysqldumpSeed) GetMetadata() (*SeedMetadata, error) {
	meta := &SeedMetadata{}
	output, err := cmd.CommandOutput(fmt.Sprintf("tail -n 100 %s", path.Join(sm.BackupDir, sm.BackupFileName)), sm.ExecWithSudo)
	if err != nil {
		sm.Logger.WithField("error", err).Info("Unable to read seed metadata")
		return meta, err
	}
	lines := cmd.OutputLines(output)
	for _, line := range lines {
		if strings.Contains(line, "GTID_PURGED") {
			meta.GtidExecuted = strings.Replace(strings.Replace(strings.Split(line, "=")[1], "'", "", -1), ";", "", -1)
		}
		if strings.Contains(line, "CHANGE MASTER") {
			meta.LogFile = strings.Replace(strings.Split(strings.Split(line, ",")[0], "=")[1], "'", "", -1)
			meta.LogPos, err = strconv.ParseInt(strings.Replace(strings.Split(strings.Split(line, ",")[1], "=")[1], ";", "", -1), 10, 64)
			if err != nil {
				sm.Logger.WithField("error", err).Info("Unable to parse seed metadata")
				return meta, err
			}
			break
		}
	}
	return meta, err
}

func (sm *MysqldumpSeed) Cleanup(side Side) {
	stage := NewSeedStage(Cleanup, sm.StatusChan)
	sm.Logger.Info("Starting cleanup")
	if side == Target {
		cleanupCmd := fmt.Sprintf("rm -rf %s", path.Join(sm.BackupDir, sm.BackupFileName))
		err := cmd.CommandRunWithFunc(cleanupCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Running cleanup", sm.StatusChan)
		})
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Cleanup failed")
			return
		}
	}
	sm.Logger.Info("Cleanup completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *MysqldumpSeed) isAvailable() bool {
	err := cmd.CommandRun("mysqldump --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}

func (sm *MysqldumpSeed) getSupportedEngines() []mysql.Engine {
	return []mysql.Engine{mysql.ROCKSDB, mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED, mysql.TokuDB}
}

func (sm *MysqldumpSeed) backupToDatadir() bool {
	return false
}
