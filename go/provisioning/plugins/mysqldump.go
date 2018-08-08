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
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/openark/golib/log"
)

type Mysqldump struct {
	Databases    []string
	BackupFolder string
	SeedID       string
}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
)

func newMysqldump(databases []string, extra ...string) (BackupPlugin, error) {
	if len(extra) < 2 {
		return nil, log.Error("Failed to initialize MySQLDump plugin. Not enought arguments")
	}
	backupFolder := extra[0]
	if _, err := os.Stat(backupFolder); err != nil {
		return nil, log.Error("Failed to initialize MySQLDump plugin. backupFolder is invalid or doesn't exists")
	}
	seedID := extra[1]
	if _, err := strconv.Atoi(seedID); err != nil {
		return nil, log.Error("Failed to initialize MySQLDump plugin. Can't parse seedID")
	}
	return Mysqldump{BackupFolder: backupFolder, Databases: databases, SeedID: seedID}, nil
}

func (m Mysqldump) Backup() error {
	var cmd string
	config.Config.RLock()
	defer config.Config.RUnlock()
	// we need to comment out SET @@GLOBAL.GTID_PURGED to be able to restore dump. Later we will issue RESET MASTER and SET @@GLOBAL.GTID_PURGED from orchestrator side
	if len(m.Databases) == 0 {
		cmd = fmt.Sprintf("mysqldump --user=%s --password=%s --port=%d --single-transaction --default-character-set=utf8mb4 --master-data=2 --routines --events --triggers --all-databases | sed -e 's/SET @@GLOBAL.GTID_PURGED=/-- SET @@GLOBAL.GTID_PURGED=/g' ",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort)
	} else {
		cmd = fmt.Sprintf("mysqldump --user=%s --password=%s --port=%d --single-transaction --default-character-set=utf8mb4 --master-data=2 --routines --events --triggers --databases %s | sed -e 's/SET @@GLOBAL.GTID_PURGED=/-- SET @@GLOBAL.GTID_PURGED=/g' ",
			config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, strings.Join(m.Databases, " "))
	}
	if config.Config.CompressLogicalBackup {
		cmd += fmt.Sprintf(" | gzip > %s", path.Join(m.BackupFolder, mysqlbackupCompressedFileName))
	} else {
		cmd += fmt.Sprintf(" > %s", path.Join(m.BackupFolder, mysqlbackupFileName))
	}
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[m.SeedID] = cmd
			log.Debug("Start backup using MySQLDump")
		})
	return log.Errore(err)
}

func (m Mysqldump) Restore() error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	if config.Config.CompressLogicalBackup {
		cmd := fmt.Sprintf("gunzip -c %s > %s", path.Join(m.BackupFolder, mysqlbackupCompressedFileName), path.Join(m.BackupFolder, mysqlbackupFileName))
		err := osagent.CommandRun(
			cmd,
			func(cmd *exec.Cmd) {
				osagent.ActiveCommands[m.SeedID] = cmd
				log.Debug("Start extracting MySQLDump backup")
			})
		if err != nil {
			return log.Errore(err)
		}
	}
	cmd := fmt.Sprintf("mysql -u%s -p%s --port %d < %s", config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, path.Join(m.BackupFolder, mysqlbackupFileName))
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[m.SeedID] = cmd
			log.Debug("Start restore using MySQLDump")
		})
	if err != nil {
		return log.Errore(err)
	}
	return err
}

func (m Mysqldump) GetMetadata() (BackupMetadata, error) {
	meta := BackupMetadata{
		BinlogCoordinates: BinlogCoordinates{}}
	file, err := os.Open(path.Join(m.BackupFolder, mysqlbackupFileName))
	if err != nil {
		return meta, log.Errore(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "GTID_PURGED") {
			meta.GTIDPurged = strings.Replace(strings.Replace(strings.Split(scanner.Text(), "=")[1], "'", "", -1), ";", "", -1)
		}
		if strings.Contains(scanner.Text(), "CHANGE MASTER") {
			meta.BinlogCoordinates.LogFile = strings.Replace(strings.Split(strings.Split(scanner.Text(), ",")[0], "=")[1], "'", "", -1)
			meta.BinlogCoordinates.LogPos, err = strconv.ParseInt(strings.Replace(strings.Split(strings.Split(scanner.Text(), ",")[1], "=")[1], ";", "", -1), 10, 64)
			if err != nil {
				return meta, log.Errore(err)
			}
			break
		}
	}
	return meta, log.Errore(err)
}
