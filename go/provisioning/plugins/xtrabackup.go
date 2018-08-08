package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/openark/golib/log"
)

type Xtrabackup struct {
	Databases    []string
	BackupFolder string
	SeedID       string
}

func newXtrabackup(databases []string, extra ...string) (BackupPlugin, error) {
	if len(extra) < 2 {
		return nil, log.Error("Failed to initialize Xtrabackup plugin. Not enought arguments")
	}
	backupFolder := extra[0]
	if _, err := os.Stat(backupFolder); err != nil {
		return nil, log.Error("Failed to initialize Xtrabackup plugin. backupFolder is invalid or doesn't exists")
	}
	seedID := extra[1]
	if _, err := strconv.Atoi(seedID); err != nil {
		return nil, log.Error("Failed to initialize Xtrabackup plugin. Can't parse seedID")
	}
	return Xtrabackup{BackupFolder: backupFolder, Databases: databases, SeedID: seedID}, nil
}

func (x Xtrabackup) Backup() error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	// if we have partial backup, we need to add mysql and sys databases
	if len(x.Databases) != 0 {
		x.Databases = addSystemDatabases(x.Databases)
	}
	cmd := fmt.Sprintf("xtrabackup --backup --user=%s --password=%s --port=%d --parallel=%d --target-dir=%s --databases='%s'",
		config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.XtrabackupParallelThreads, x.BackupFolder, strings.Join(x.Databases, " "))
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[x.SeedID] = cmd
			log.Debug("Start backup using Xtrabackup")
		})
	return log.Errore(err)
}

func (x Xtrabackup) Restore() error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	return restoreXtrabackup(x.SeedID, x.BackupFolder, x.Databases)
}

func (x Xtrabackup) GetMetadata() (BackupMetadata, error) {
	return parseXtrabackupMetadata(x.BackupFolder)
}
