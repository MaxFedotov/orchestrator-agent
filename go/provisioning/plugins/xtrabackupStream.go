package plugins

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/openark/golib/log"
)

type XtrabackupStream struct {
	Databases    []string
	BackupFolder string
	SeedID       string
	TargetHost   string
}

func newXtrabackupStream(databases []string, extra ...string) (BackupPlugin, error) {
	if len(extra) < 3 {
		return nil, log.Error("Failed to initialize Xtrabackup-stream plugin. Not enought arguments")
	}
	backupFolder := extra[0]
	if _, err := os.Stat(backupFolder); err != nil {
		return nil, log.Error("Failed to initialize Xtrabackup plugin. backupFolder is invalid or doesn't exists")
	}
	seedID := extra[1]
	if _, err := strconv.Atoi(seedID); err != nil {
		return nil, log.Error("Failed to initialize Xtrabackup plugin. Can't parse seedID")
	}
	targetHost := extra[2]
	/* UNCOMENT THIS WHEN SWITCH FROM NC TO CUSTOM TCP SERVER
	if _, err := net.Dial("tcp", targetHost+":"+string(config.Config.SeedPort)); err != nil {
		return nil, log.Error("Failed to initialize Xtrabackup-stream plugin. Can't connect to targetHost")
	}
	*/
	return XtrabackupStream{BackupFolder: backupFolder, Databases: databases, SeedID: seedID, TargetHost: targetHost}, nil
}

func (xs XtrabackupStream) Backup() error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	// if we have partial backup, we need to add mysql and sys databases
	if len(xs.Databases) != 0 {
		xs.Databases = addSystemDatabases(xs.Databases)
	}
	cmd := fmt.Sprintf("innobackupex %s --stream=xbstream --user=%s --password=%s --port=%d --parallel=%d --databases='%s' | nc -w 20 %s %d",
		config.Config.MySQLBackupDir, config.Config.MySQLTopologyUser, config.Config.MySQLTopologyPassword, config.Config.MySQLPort, config.Config.XtrabackupParallelThreads, strings.Join(xs.Databases, " "), xs.TargetHost, config.Config.SeedPort)
	if runtime.GOOS == "darwin" {
		cmd += " -c"
	}
	err := osagent.CommandRun(
		cmd,
		func(cmd *exec.Cmd) {
			osagent.ActiveCommands[xs.SeedID] = cmd
			log.Debug("Start backup using Xtrabackup-stream")
		})
	return log.Errore(err)
}

func (xs XtrabackupStream) Restore() error {
	config.Config.RLock()
	defer config.Config.RUnlock()
	return restoreXtrabackup(xs.SeedID, xs.BackupFolder, xs.Databases)
}

func (xs XtrabackupStream) GetMetadata() (BackupMetadata, error) {
	return parseXtrabackupMetadata(xs.BackupFolder)
}
