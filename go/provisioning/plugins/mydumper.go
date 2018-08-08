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

type Mydumper struct {
	Databases    []string
	BackupFolder string
	SeedID       string
}

const (
	mydumperMetadataFile = "metadata"
)

func newMydumper(databases []string, extra ...string) (BackupPlugin, error) {
	if len(extra) < 2 {
		return nil, log.Error("Failed to initialize MyDumper plugin. Not enought arguments")
	}
	backupFolder := extra[0]
	if _, err := os.Stat(backupFolder); err != nil {
		return nil, log.Error("Failed to initialize MyDumper plugin. backupFolder is invalid or doesn't exists")
	}
	seedID := extra[1]
	if _, err := strconv.Atoi(seedID); err != nil {
		return nil, log.Error("Failed to initialize MyDumper plugin. Can't parse seedID")
	}
	return Mydumper{BackupFolder: backupFolder, Databases: databases, SeedID: seedID}, nil
}

func (md Mydumper) Backup() error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	cmd := fmt.Sprintf("mydumper --user=%s --password=%s --port=%d --threads=%d --outputdir=%s --triggers --events --routines --regex='(%s)'",
		config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.MyDumperParallelThreads, md.BackupFolder, strings.Join(md.Databases, "\\.|")+"\\.")
	if config.Config.CompressLogicalBackup {
		cmd += fmt.Sprintf(" --compress")
	}
	if config.Config.MyDumperRowsChunkSize != 0 {
		cmd += fmt.Sprintf(" --rows=%d", config.Config.MyDumperRowsChunkSize)
	}
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[md.SeedID] = cmd
			log.Debug("Start backup using MyDumper")
		})
	return log.Errore(err)
}

func (md Mydumper) Restore() error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	//mydumper doesn't set sql_mode correctly, so we will do the same way as mysqldump does. Remember old sql_mode, than set it to
	//NO_AUTO_VALUE_ON_ZERO and then set it back
	sqlMode, err := dbagent.GetMySQLSql_mode()
	if err != nil {
		return log.Errore(err)
	}
	if err := dbagent.SetMySQLSql_mode("NO_AUTO_VALUE_ON_ZERO"); err != nil {
		return log.Errore(err)
	}
	cmd := fmt.Sprintf("myloader -u %s -p %s -o --port %d -t %d -d %s",
		config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.MyDumperParallelThreads, md.BackupFolder)
	err = osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[md.SeedID] = cmd
			log.Debug("Start restore using Mydumper")
		})
	if err != nil {
		return log.Errore(err)
	}
	err = dbagent.SetMySQLSql_mode(sqlMode)
	return log.Errore(err)
}

func (md Mydumper) GetMetadata() (BackupMetadata, error) {
	meta := BackupMetadata{
		BinlogCoordinates: BinlogCoordinates{}}
	file, err := os.Open(path.Join(md.BackupFolder, mydumperMetadataFile))
	if err != nil {
		return meta, log.Errore(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "Log:") {
			meta.BinlogCoordinates.LogFile = strings.Trim(strings.Split(scanner.Text(), ":")[1], " ")
		}
		if strings.Contains(scanner.Text(), "Pos:") {
			meta.BinlogCoordinates.LogPos, err = strconv.ParseInt(strings.Trim(strings.Split(scanner.Text(), ":")[1], " "), 10, 64)
			if err != nil {
				return meta, log.Errore(err)
			}
		}
		if strings.Contains(scanner.Text(), "GTID:") {
			meta.GTIDPurged = strings.Trim(strings.SplitAfterN(scanner.Text(), ":", 2)[1], " ")
			break
		}
	}
	return meta, log.Errore(err)
}
