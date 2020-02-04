package seed

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
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
	mysqlbackupFileName = "orchestrator_agent_backup.sql"
)

func (sm *MysqldumpSeed) Prepare(side Side) {
	stage := NewSeedStage(Prepare, sm.StatusChan)
	stage.UpdateSeedStatus(Completed, nil, "Stage completed")
}

func (sm *MysqldumpSeed) Backup(seedHost string, mysqlPort int) {
	stage := NewSeedStage(Backup, sm.StatusChan)
	backupCmd := fmt.Sprintf("mysqldump --host=%s --user=%s --password=%s --port=%d --single-transaction --default-character-set=utf8mb4 --master-data=2 --routines --events --triggers --all-databases", seedHost, sm.SeedUser, sm.SeedPassword, mysqlPort)
	if sm.Config.Compress {
		backupCmd += fmt.Sprintf(" -C")
	}
	backupCmd += fmt.Sprintf(" > %s", path.Join(sm.BackupDir, mysqlbackupFileName))
	sm.Logger.Info("Starting backup")
	err := cmd.CommandRunWithFunc(backupCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Running backup")
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error())
		sm.Logger.WithField("error", err).Info("Backup failed")
		return
	}
	sm.Logger.Info("Backup completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed")

}

func (sm *MysqldumpSeed) Restore() {
	stage := NewSeedStage(Restore, sm.StatusChan)
	restoreCmd := fmt.Sprintf("cat %s | mysql -u%s -p%s --port %d", path.Join(sm.BackupDir, mysqlbackupFileName), sm.SeedUser, sm.SeedPassword, sm.MySQLPort)
	sm.Logger.Info("Starting restore")
	err := cmd.CommandRunWithFunc(restoreCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Running restore")
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error())
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	sm.Logger.Info("Restore completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed")
}

func (sm *MysqldumpSeed) GetMetadata() (*BackupMetadata, error) {
	meta := &BackupMetadata{}
	file, err := os.Open(path.Join(sm.BackupDir, mysqlbackupFileName))
	if err != nil {
		return meta, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "GTID_PURGED") {
			meta.GtidExecuted = strings.Replace(strings.Replace(strings.Split(scanner.Text(), "=")[1], "'", "", -1), ";", "", -1)
		}
		if strings.Contains(scanner.Text(), "CHANGE MASTER") {
			meta.LogFile = strings.Replace(strings.Split(strings.Split(scanner.Text(), ",")[0], "=")[1], "'", "", -1)
			meta.LogPos, err = strconv.ParseInt(strings.Replace(strings.Split(strings.Split(scanner.Text(), ",")[1], "=")[1], ";", "", -1), 10, 64)
			if err != nil {
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
		cleanupCmd := fmt.Sprintf("rm -rf %s", path.Join(sm.BackupDir, mysqlbackupFileName))
		err := cmd.CommandRunWithFunc(cleanupCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Running cleanup")
		})
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error())
			sm.Logger.WithField("error", err).Info("Cleanup failed")
			return
		}
	}
	sm.Logger.Info("Cleanup completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed")
}

func (sm *MysqldumpSeed) IsAvailable() bool {
	err := cmd.CommandRun("mysqldump --version", sm.ExecWithSudo)
	if err != nil {
		return false
	}
	return true
}
