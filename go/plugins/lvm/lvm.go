package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"time"

	"github.com/github/orchestrator-agent/go/functions"
	"github.com/openark/golib/log"
	"github.com/openark/golib/sqlutils"
)

type lvm struct {
	engines           []string
	databaseSelection bool
	availiableRestore bool
}

type config struct {
	Plugins struct {
		LVM struct {
			SnapshotSize          string `json:"SnapshotSize"`
			SnapshotName          string `json:"SnapshotName"`
			SnapshotVolumeGroup   string `json:"SnapshotVolumeGroup"`
			SnapshotLogicalVolume string `json:"SnapshotLogicalVolume"`
		}
	}
}

var BackupPlugin = lvm{
	engines:           []string{"InnoDB", "MyISAM", "ROCKSDB", "TokuDB"},
	databaseSelection: false,
	availiableRestore: true,
}

var Config *config
var SnapshotName string

const (
	snapshot     = "orchestrator_seed"
	metadataName = "binlog_info.json"
)

func init() {
	var configFiles = [3]string{"/etc/orchestrator-agent.conf.json", "/conf/orchestrator-agent.conf.json", "orchestrator-agent.conf.json"}
	for _, fileName := range configFiles {
		file, err := os.Open(fileName)
		if err == nil {
			decoder := json.NewDecoder(file)
			err := decoder.Decode(&Config)
			if err == nil {
				log.Infof("LVM Backup plugin - read config: %s", fileName)
			} else {
				log.Infof("LVM Backup plugin - cannot read config file: %s, %s", fileName, err)
			}
		}
	}
	SnapshotName = Config.Plugins.LVM.SnapshotName + "_seed"
}

func isMounted(mountPoint string) bool {
	output, err := functions.CommandOutput(fmt.Sprintf("grep %s /etc/mtab", mountPoint))
	_, err = functions.OutputTokens(`[ \t]+`, output, err)
	if err != nil {
		// when grep does not find rows, it returns an error. So this is actually OK
		return false
	}
	return true
}

func isAvailiable(volumeGroup string) bool {
	_, err := functions.CommandOutput(fmt.Sprintf("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent,time --sort -time %s", volumeGroup))
	if err != nil {
		return false
	}
	return true
}

func saveMetadata(db *sql.DB, mysqlDatadir string) error {
	metadata := functions.BackupMetadata{}
	err := sqlutils.QueryRowsMap(db, "SHOW MASTER STATUS;", func(m sqlutils.RowMap) error {
		metadata.LogFile = m.GetString("File")
		metadata.LogPos = m.GetInt64("Position")
		metadata.GtidExecuted = m.GetString("Executed_Gtid_Set")
		return nil
	})
	if err != nil {
		return err
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filepath.Join(mysqlDatadir, metadataName), metadataJSON, 0644)
	return err
}

func createSnapshot(mysqlUser string, mysqlPassword string, mysqlPort int, mysqlDatadir string, snapshot string) error {
	db, err := functions.OpenConnection(mysqlUser, mysqlPassword, mysqlPort)
	if err != nil {
		return fmt.Errorf("unable to connect to MySQL: %+v", err)
	}
	query := "FLUSH TABLES WITH READ LOCK;"
	_, err = sqlutils.ExecNoPrepare(db, query)
	if err != nil {
		return fmt.Errorf("unable to execute query %s: %+v", query, err)
	}
	err = saveMetadata(db, mysqlDatadir)
	if err != nil {
		err = fmt.Errorf("unable to save backup metadata: %+v", err)
		goto Unlock
	}
	err = functions.CreateSnapshot(Config.Plugins.LVM.SnapshotSize, SnapshotName, Config.Plugins.LVM.SnapshotVolumeGroup, Config.Plugins.LVM.SnapshotLogicalVolume)
	if err != nil {
		err = fmt.Errorf("unable to execute create snapshot command: %+v", err)
		goto Unlock
	}
Unlock:
	_, err = sqlutils.ExecNoPrepare(db, "UNLOCK TABLES;")
	return err
}

func (l lvm) Backup(params functions.AgentParams, databases []string, errs chan error) io.Reader {
	SnapshotName = snapshot + "_" + string(time.Now().Format("2006_01_02_15_04_05"))
	var stderr bytes.Buffer
	var data io.Reader
	if isMounted(params.BackupFolder) {
		log.Warningf("LVM Backup plugin - volume already mounted on source host %s. Unmouting", params.BackupFolder)
		err := functions.Unmount(params.BackupFolder)
		if err != nil {
			errs <- fmt.Errorf("LVM Backup plugin - unable to unmount %s: %+v", params.BackupFolder, err)
			return data
		}
	}
	err := createSnapshot(params.MysqlUser, params.MysqlPassword, params.MysqlPort, params.MysqlDatadir, SnapshotName)
	if err != nil {
		errs <- fmt.Errorf("LVM Backup plugin - unable to create snapshot: %+v", err)
		return data
	}
	err = functions.MountLV(params.BackupFolder, fmt.Sprintf("/dev/%s/%s", Config.Plugins.LVM.SnapshotVolumeGroup, SnapshotName))
	if err != nil {
		errs <- fmt.Errorf("LVM Backup plugin - unable to mount snapshot: %+v", err)
		return data
	}
	cmd := exec.Command("tar", "cf", "-", "-C", params.BackupFolder, ".")
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		errs <- fmt.Errorf("LVM Backup plugin - unable to prepare pipe for backup: %+v", err)
		return data
	}
	go func() {
		err = cmd.Start()
		if err != nil {
			errs <- fmt.Errorf("LVM Backup plugin - unable to start backup: %+v", err)
		}
		err = cmd.Wait()
		if err != nil {
			errs <- fmt.Errorf("LVM Backup plugin - unable to backup: %+v", err)
		}
	}()
	return out
}

func (l lvm) Restore(params functions.AgentParams) error {
	mysqlOSuser, err := user.Lookup("mysql")
	if err != nil {
		return fmt.Errorf("LVM Backup plugin - unable to find uid for mysql user: %+v", err)
	}
	mysqlUID, err := strconv.Atoi(mysqlOSuser.Uid)
	if err != nil {
		return fmt.Errorf("LVM Backup plugin - unable to parse uid for mysql user: %+v", err)
	}
	mysqlOSGroup, err := user.LookupGroup("mysql")
	if err != nil {
		return fmt.Errorf("LVM Backup plugin - unable to find gid for mysql group: %+v", err)
	}
	mysqlGID, err := strconv.Atoi(mysqlOSGroup.Gid)
	if err != nil {
		return fmt.Errorf("LVM Backup plugin - unable to parse gid for mysql group: %+v", err)
	}
	err = functions.ChownRecurse(params.MysqlDatadir, mysqlUID, mysqlGID)
	if err != nil {
		return fmt.Errorf("LVM Backup plugin - unable to change owner to mysql:mysql for %s : %+v", params.MysqlDatadir, err)
	}
	err = functions.MySQLStart()
	if err != nil {
		return fmt.Errorf("LVM Backup plugin - unable to start MySQL: %+v", err)
	}
	return err
}

func (l lvm) GetMetadata(params functions.AgentParams) (functions.BackupMetadata, error) {
	backupMetadata := functions.BackupMetadata{}
	metadata, err := os.Open(filepath.Join(params.MysqlDatadir, metadataName))
	if err != nil {
		return backupMetadata, fmt.Errorf("LVM Backup plugin - unable to read metadata from %s: %+v", filepath.Join(params.MysqlDatadir, metadataName), err)
	}
	decoder := json.NewDecoder(metadata)
	err = decoder.Decode(&backupMetadata)
	if err != nil {
		return backupMetadata, fmt.Errorf("LVM Backup plugin - unable to parse metadata from %s: %+v", filepath.Join(params.MysqlDatadir, metadataName), err)
	}
	return backupMetadata, err
}

func (l lvm) Receive(params functions.AgentParams, data io.Reader) error {
	cmd := exec.Command("tar", "-xf", "-", "-C", params.MysqlDatadir)
	cmd.Stdin = data
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("LVM Backup plugin - unable to start unpack: %+v", err)
	}
	return cmd.Wait()
}

func (l lvm) Prepare(params functions.AgentParams, hostType string) error {
	switch hostType {
	case "target":
		{
			err := functions.MySQLStop()
			if err != nil {
				return fmt.Errorf("LVM Backup plugin - unable to stop MySQL: %+v", err)
			}
			err = functions.DeleteDirContents(params.MysqlDatadir)
			if err != nil {
				return fmt.Errorf("LVM Backup plugin - unable to remove MySQL datadir %s: %+v", params.MysqlDatadir, err)
			}
			return nil
		}
	case "source":
		{
			return nil
		}
	default:
		{
			return nil
		}
	}
}

func (l lvm) Cleanup(params functions.AgentParams, hostType string) error {
	switch hostType {
	case "target":
		{
			return nil
		}
	case "source":
		{
			err := functions.Unmount(params.BackupFolder)
			if err != nil {
				return fmt.Errorf("LVM Backup plugin - unable to perform cleanup, unmount error: %+v", err)
			}
			err = functions.RemoveLV(fmt.Sprintf("/dev/%s/%s", Config.Plugins.LVM.SnapshotVolumeGroup, SnapshotName))
			if err != nil {
				return fmt.Errorf("LVM Backup plugin - unable to perform cleanup, cannot delete snapshot, error: %+v", err)
			}
			return nil
		}
	default:
		{
			return nil
		}
	}
}

func (l lvm) SupportedEngines() []string {
	return l.engines
}

func (l lvm) IsAvailiableBackup() bool {
	if ok := isAvailiable(Config.Plugins.LVM.SnapshotVolumeGroup); !ok {
		log.Errorf("LVM Backup plugin - LVM not configured or volume group %s not found. Plugin will be marked as unavailiable", Config.Plugins.LVM.SnapshotVolumeGroup)
		return false
	}
	if Config.Plugins.LVM.SnapshotSize == "" || Config.Plugins.LVM.SnapshotVolumeGroup == "" || Config.Plugins.LVM.SnapshotLogicalVolume == "" {
		log.Error("LVM Backup plugin - Plugin configuration not found. Plugin will be marked as unavailiable")
		return false
	}
	return true
}

func (l lvm) IsAvailiableRestore() bool {
	return l.availiableRestore
}

func (l lvm) SupportDatabaseSelection() bool {
	return l.databaseSelection
}

func main() {

}
