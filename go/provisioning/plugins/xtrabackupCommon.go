// Common functions used in both, xtrabackup and xtrabackup-stream

package plugins

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/dbagent"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/openark/golib/log"
)

const (
	xtrabackupMetadataFile  = "xtrabackup_binlog_info"
	mysqlUserBackupFileName = "mysql_users_backup.sql"
)

func copyXtrabackup(seedID string, backupFolder string) error {
	cmd := fmt.Sprintf("xtrabackup --copy-back --target-dir=%s", backupFolder)
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[seedID] = cmd
			log.Debug("Start copying xtrabackup to datadir")
		})
	return err
}

func prepareXtrabackup(seedID string, backupFolder string) error {
	cmd := fmt.Sprintf("xtrabackup --prepare --target-dir=%s", backupFolder)
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[seedID] = cmd
			log.Debug("Start preparing xtrabackup")
		})
	return err
}

func runMySQLUpgrade(seedID string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd := fmt.Sprintf("mysql_upgrade --protocol=tcp -u%s -p%s --port %d --force", config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort)
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[seedID] = cmd
			log.Debug("Start mysql_upgrade")
		})
	return err
}

func restoreXtrabackup(seedID string, backupFolder string, databases []string) (err error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	var MySQLInnoDBLogDir string
	if len(config.Config.MySQLInnoDBLogDir) == 0 {
		MySQLInnoDBLogDir = config.Config.MySQLDataDir
	} else {
		MySQLInnoDBLogDir = config.Config.MySQLInnoDBLogDir
	}
	if err := prepareXtrabackup(seedID, backupFolder); err != nil {
		return log.Errore(err)
	}
	// xtrabackup full\partial, xtrabackup-stream full\partial to MySQLBackupDir
	if backupFolder != config.Config.MySQLDataDir {
		if err := osagent.MySQLStop(); err != nil {
			return log.Errore(err)
		}
		if err := osagent.DeleteFile(MySQLInnoDBLogDir, "ib_logfile*"); err != nil {
			return log.Errore(err)
		}
		if err := osagent.DeleteDirContents(config.Config.MySQLDataDir); err != nil {
			return log.Errore(err)
		}
		if err := copyXtrabackup(seedID, backupFolder); err != nil {
			return log.Errore(err)
		}
		if err := osagent.DeleteFile(MySQLInnoDBLogDir, "ib_logfile*"); err != nil {
			return log.Errore(err)
		}
	}
	if err := osagent.ChangeDatadirPermissions(seedID); err != nil {
		return log.Errore(err)
	}
	if err := osagent.MySQLStart(); err != nil {
		return log.Errore(err)
	}
	if len(databases) > 0 {
		if err := runMySQLUpgrade(seedID); err != nil {
			return log.Errore(err)
		}
		if err := osagent.MySQLRestart(); err != nil {
			return log.Errore(err)
		}
	}
	// restore users
	if err := restoreMySQLUsers(seedID); err != nil {
		return log.Errore(err)
	}
	return err
}

func parseXtrabackupMetadata(backupFolder string) (BackupMetadata, error) {
	meta := BackupMetadata{
		BinlogCoordinates: BinlogCoordinates{}}
	var params []string
	file, err := os.Open(path.Join(backupFolder, xtrabackupMetadataFile))
	if err != nil {
		return meta, log.Errore(err)
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	metadata, err := reader.ReadString('\n')
	if err != nil {
		return meta, log.Errore(err)
	}
	params = strings.Split(metadata, "\t")
	meta.BinlogCoordinates.LogFile = params[0]
	meta.BinlogCoordinates.LogPos, err = strconv.ParseInt(strings.Trim(params[1], "\n"), 10, 64)
	if err != nil {
		return meta, log.Errore(err)
	}
	if len(params) > 2 {
		meta.GTIDPurged = strings.Trim(params[2], "\n")
	}
	return meta, err
}

func restoreMySQLUsers(seedID string) error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd := fmt.Sprintf("mysql -u%s -p%s --port %d mysql < %s", config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, path.Join(config.Config.MySQLBackupDir, mysqlUserBackupFileName))
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[seedID] = cmd
			log.Debug("Restoring MySQL users")
		})
	return log.Errore(err)
}

func addSystemDatabases(databases []string) []string {
	//if we don't already have mysql database in databases, add it
	if !osagent.Contains("mysql", databases) {
		databases = append(databases, "mysql")
	}
	//and if we are on 5.7 we need to add sys db because sometimes during mysql_upgrade run we can get errors like mysql_upgrade: [ERROR] 1813: Tablespace '`sys`.`sys_config`' exists
	version, _ := dbagent.GetMySQLVersion()
	if version == "5.7" && !osagent.Contains("sys", databases) {
		databases = append(databases, "sys")
	}
	return databases
}
