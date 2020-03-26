package seed

import (
	"fmt"
	"path"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/helper/mysql"
	"github.com/openark/golib/sqlutils"
	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
)

var defaultMydumperOpts = map[string]bool{
	"--host":             true,
	"-h":                 true,
	"--user":             true,
	"-u":                 true,
	"--port":             true,
	"-P":                 true,
	"--password":         true,
	"-p":                 true,
	"-o":                 true,
	"--outputdir":        true,
	"--overwrite-tables": true,
	"-d":                 true,
	"--directory":        true,
	"--no-backup-locks":  true,
}

type MydumperSeed struct {
	*Base
	*MethodOpts
	Config           *MydumperConfig
	Logger           *log.Entry
	BackupFolderName string
	MetadataFileName string
}

type MydumperConfig struct {
	Enabled                bool     `toml:"enabled"`
	MydumperAdditionalOpts []string `toml:"mydumper-addtional-opts"`
	MyloaderAdditionalOpts []string `toml:"myloader-additional-opts"`
}

func (sm *MydumperSeed) Prepare(side Side) {
	stage := NewSeedStage(Prepare, sm.StatusChan)
	sm.Logger.Info("Starting prepare")
	if side == Target {
		cleanupCmd := fmt.Sprintf("rm -rf %s", path.Join(sm.BackupDir, sm.BackupFolderName))
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

func (sm *MydumperSeed) Backup(seedHost string, mysqlPort int) {
	stage := NewSeedStage(Backup, sm.StatusChan)
	var addtionalOpts []string
	for _, opt := range sm.Config.MydumperAdditionalOpts {
		if defaultMydumperOpts[strings.Split(opt, " ")[0]] {
			sm.Logger.WithField("MydumperOption", opt).Error("Will skip mydumper option, as it is already used by default")
		} else {
			addtionalOpts = append(addtionalOpts, opt)
		}
	}
	// add --no-backup-locks to mydumper, as they are removed in MySQL 8 and cause mydumper errors
	backupCmd := fmt.Sprintf("mydumper --no-backup-locks --host %s --user %s --password %s --port %d --outputdir %s %s", seedHost, sm.User, sm.Password, mysqlPort, path.Join(sm.BackupDir, sm.BackupFolderName), strings.Join(addtionalOpts, " "))
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

func (sm *MydumperSeed) Restore() {
	stage := NewSeedStage(Restore, sm.StatusChan)
	sm.Logger.Info("Starting restore")
	var addtionalOpts []string
	for _, opt := range sm.Config.MyloaderAdditionalOpts {
		if defaultMydumperOpts[strings.Split(opt, " ")[0]] {
			sm.Logger.WithField("MyloaderOption", opt).Error("Will skip myloader option, as it is already used by default")
		} else {
			addtionalOpts = append(addtionalOpts, opt)
		}
	}
	// https://github.com/maxbube/mydumper/issues/142
	var sqlMode string
	if err := mysql.QueryData(sm.MySQLClient.Conn, "SELECT @@sql_mode;", nil, func(m sqlutils.RowMap) error {
		sqlMode = m.GetString("PLUGIN_STATUS")
		return nil
	}); err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	if err := mysql.Exec(sm.MySQLClient.Conn, "SET GLOBAL SQL_MODE='NO_AUTO_VALUE_ON_ZERO'"); err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	restoreCmd := fmt.Sprintf("myloader --host localhost --user %s --password %s --port %d --directory %s --overwrite-tables %s", sm.User, sm.Password, sm.MySQLPort, path.Join(sm.BackupDir, sm.BackupFolderName), strings.Join(addtionalOpts, " "))

	err := cmd.CommandRunWithFunc(restoreCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Running restore", sm.StatusChan)
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL SQL_MODE='%s'", sqlMode)); err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	sm.Logger.Info("Restore completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *MydumperSeed) GetMetadata() (*SeedMetadata, error) {
	meta := &SeedMetadata{}
	output, err := cmd.CommandOutput(fmt.Sprintf("cat %s", path.Join(sm.BackupDir, sm.BackupFolderName, sm.MetadataFileName)), sm.ExecWithSudo)
	if err != nil {
		sm.Logger.WithField("error", err).Info("Unable to read seed metadata")
		return meta, err
	}
	lines := cmd.OutputLines(output)
	for _, line := range lines {
		if strings.Contains(line, "Log:") {
			meta.LogFile = strings.Trim(strings.Split(line, ":")[1], " ")
		}
		if strings.Contains(line, "Pos:") {
			meta.LogPos, err = strconv.ParseInt(strings.Trim(strings.Split(line, ":")[1], " "), 10, 64)
			if err != nil {
				sm.Logger.WithField("error", err).Info("Unable to parse seed metadata")
				return meta, err
			}
		}
		if strings.Contains(line, "GTID:") {
			meta.GtidExecuted = strings.Trim(strings.SplitAfterN(line, ":", 2)[1], " ")
			break
		}
	}
	return meta, err
}

func (sm *MydumperSeed) Cleanup(side Side) {
	stage := NewSeedStage(Cleanup, sm.StatusChan)
	sm.Logger.Info("Starting cleanup")
	if side == Target {
		cleanupCmd := fmt.Sprintf("rm -rf %s", path.Join(sm.BackupDir, sm.BackupFolderName))
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
