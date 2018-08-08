package provisioning

import (
	"fmt"
	"os/exec"
	"path"
	"reflect"

	"github.com/github/orchestrator-agent/go/config"
	"github.com/github/orchestrator-agent/go/dbagent"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/github/orchestrator-agent/go/provisioning/plugins"
	"github.com/openark/golib/log"
)

const (
	mysqlBackupDatadirName = "mysql_datadir_backup.tar.gz"
)

var activeSeeds = make(map[string]plugins.BackupPlugin)

func StartSeed(seedID string, seedMethod string, backupFolder string, databases []string, targetHost string) (err error) {
	if _, ok := plugins.SeedMethods[seedMethod]; !ok {
		return log.Error("Failed to start seed. Invalid seedMethod")
	}
	// if we optionally pass some databases, let's check that they exists
	if len(databases) != 0 {
		availiableDatabases, _ := dbagent.GetMySQLDatabases()
		for _, db := range databases {
			if !osagent.Contains(db, availiableDatabases) {
				return log.Errorf("Cannot backup database %+v. Database doesn't exists", db)
			}
		}

	}
	plugin, err := plugins.IntializePlugin(seedMethod, databases, backupFolder, seedID, targetHost)
	if err == nil {
		if _, ok := activeSeeds[seedID]; ok {
			return log.Error("Failed to start seed. SeedID already exist")
		}
		activeSeeds[seedID] = plugin
	}
	return log.Errore(err)
}

func StartBackup(seedID string) (err error) {
	if _, ok := activeSeeds[seedID]; !ok {
		return log.Error("Failed to start backup. SeedID doesn't exist")
	}
	plugin := activeSeeds[seedID]
	err = plugin.Backup()
	return log.Errore(err)
}

func GetBackupMetadata(seedID string) (plugins.BackupMetadata, error) {
	var meta plugins.BackupMetadata
	var err error
	if _, ok := activeSeeds[seedID]; !ok {
		return meta, log.Error("Failed to return backup metadata. SeedID doesn't exist")
	}
	plugin := activeSeeds[seedID]
	meta, err = plugin.GetMetadata()
	return meta, err
}

func StartRestore(seedID string) (err error) {
	config.Config.RLock()
	defer config.Config.RUnlock()
	if _, ok := activeSeeds[seedID]; !ok {
		return log.Error("Failed to start restore. SeedID doesn't exist")
	}
	plugin := activeSeeds[seedID]
	databases := reflect.ValueOf(plugin).Field(0).Interface().([]string)
	//if we choose to backup only specific databases, add them to my.cnf replicate-do-db and restart MySQL
	if len(databases) > 0 {
		for _, db := range databases {
			if err := config.AddKeyToMySQLConfig("replicate-do-db", db); err != nil {
				return log.Errore(err)
			}
		}
		if err := osagent.MySQLRestart(); err != nil {
			return log.Errore(err)
		}
	}
	err = plugin.Restore()
	if err == nil {
		// just execute CHANGE MASTER TO in order to save replication user and password. All other will be done by orchestrator
		if err := dbagent.SetReplicationUserAndPassword(); err != nil {
			return log.Errore(err)
		}
	}
	// if we backed up old datadir and have errors during restore process, let's remove contents of datadir and move back old datadir
	if err != nil && config.Config.MySQLBackupOldDatadir {
		// stop MySQL
		if err := osagent.MySQLStop(); err != nil {
			return log.Errore(err)
		}
		if err := osagent.DeleteDirContents(config.Config.MySQLDataDir); err != nil {
			return log.Errore(err)
		}
		cmd := fmt.Sprintf("tar zxfp %s -C %s", path.Join(config.Config.MySQLBackupDir, mysqlBackupDatadirName), config.Config.MySQLDataDir)
		err := osagent.CommandRun(
			cmd,
			func(cmd *exec.Cmd) {
				osagent.ActiveCommands[seedID] = cmd
				log.Debugf("Start restoring old datadir")
			})
		if err != nil {
			return log.Errore(err)
		}
		if err := osagent.MySQLStart(); err != nil {
			return log.Errore(err)
		}
	}
	return log.Errore(err)
}
