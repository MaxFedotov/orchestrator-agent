package seed

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/github/orchestrator-agent/go/helper/mysql"
	"github.com/github/orchestrator-agent/go/osagent"
	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
)

type LVMSeed struct {
	*Base
	*MethodOpts
	Config           *LVMConfig
	Logger           *log.Entry
	MetadataFileName string
}

type LVMConfig struct {
	Enabled                            bool   `toml:"enabled"`
	CreateSnapshotCommand              string `toml:"create-snapshot-command"`
	AvailableLocalSnapshotHostsCommand string `toml:"available-local-snapshot-hosts-command"`
	AvailableSnapshotHostsCommand      string `toml:"available-snapshot-hosts-command"`
	SnapshotVolumesFilter              string `toml:"snapshot-volumes-filter"`
	SnapshotMountPoint                 string `toml:"snapshot-mount-point"`
	CreateNewSnapshotForSeed           bool   `toml:"create-new-snapshot-for-seed"`
	SocatUseSSL                        bool   `toml:"socat-use-ssl"`
	SocatSSLCertFile                   string `toml:"socat-ssl-cert-file"`
	SocatSSLCAFile                     string `toml:"socat-ssl-cat-file"`
	SocatSSLSkipVerify                 bool   `toml:"socat-ssl-skip-verify"`
}

func (sm *LVMSeed) Prepare(side Side) {
	stage := NewSeedStage(Prepare, sm.StatusChan)
	sm.Logger.Info("Starting prepare")
	if side == Source {
		var latestSnapshotTime time.Time
		var latestLV *osagent.LogicalVolume
		if sm.Config.CreateNewSnapshotForSeed {
			if err := osagent.CreateSnapshot(sm.Config.CreateSnapshotCommand, sm.Cmd); err != nil {
				stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
				sm.Logger.WithField("error", err).Info("Failed to create snapshot")
				return
			}
		}
		logicalVolumes, err := osagent.GetLogicalVolumes("", sm.Config.SnapshotVolumesFilter, sm.Cmd)
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Failed to get logical volumes info")
			return
		}
		for _, lv := range logicalVolumes {
			if lv.IsSnapshot == true {
				if lv.CreatedAt.After(latestSnapshotTime) {
					latestSnapshotTime = lv.CreatedAt
					latestLV = lv
				}
			}
		}
		if _, err := osagent.MountLV(sm.BackupDir, latestLV.Path, sm.Cmd); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Failed to mount snapshot")
			return
		}
	}
	if side == Target {
		var wg sync.WaitGroup
		stage.UpdateSeedStatus(Running, nil, "Stopping MySQL", sm.StatusChan)
		if err := osagent.MySQLStop(sm.MySQLServiceStopCommand, sm.Cmd); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		cleanupDatadirCmd := fmt.Sprintf("find %s -mindepth 1 -regex ^.*$ -delete", sm.MySQLDatadir)
		err := sm.Cmd.CommandRunWithFunc(cleanupDatadirCmd, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Cleaning MySQL datadir", sm.StatusChan)
		})
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			socatConOpts := fmt.Sprintf("TCP-LISTEN:%d,reuseaddr", sm.SeedPort)
			if sm.Config.SocatUseSSL {
				socatConOpts = fmt.Sprintf("openssl-listen:%d,reuseaddr,cert=%s", sm.SeedPort, sm.Config.SocatSSLCertFile)
				if len(sm.Config.SocatSSLCAFile) > 0 {
					socatConOpts += fmt.Sprintf(",cafile=%s", sm.Config.SocatSSLCAFile)
				}
				if sm.Config.SocatSSLSkipVerify {
					socatConOpts += ",verify=0"
				}
			}
			recieveCmd := fmt.Sprintf("socat -u %s EXEC:\"tar xzf - -C %s\"", socatConOpts, sm.MySQLDatadir)
			err := sm.Cmd.CommandRunWithFunc(recieveCmd, func(cmd *pipe.State) {
				stage.UpdateSeedStatus(Running, cmd, fmt.Sprintf("Started socat on port %d. Waiting for backup data", sm.SeedPort), sm.StatusChan)
				wg.Done()
			})
			if err != nil {
				stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
				sm.Logger.WithField("error", err).Info("Socat failed")
				return
			}
		}(&wg)
		wg.Wait()
		sm.Logger.Info("Prepare completed")
		stage.UpdateSeedStatus(Completed, nil, fmt.Sprintf("Prepare completed. Started socat on port %d. Waiting for backup data", sm.SeedPort), sm.StatusChan)
		return
	}
	sm.Logger.Info("Prepare completed")
	stage.UpdateSeedStatus(Completed, nil, "Prepare completed", sm.StatusChan)
}

func (sm *LVMSeed) Backup(seedHost string, mysqlPort int) {
	stage := NewSeedStage(Backup, sm.StatusChan)
	socatConOpts := fmt.Sprintf("TCP:%s:%d", seedHost, sm.SeedPort)
	if sm.Config.SocatUseSSL {
		socatConOpts = fmt.Sprintf("openssl-connect:%s:%d,cert=%s", seedHost, sm.SeedPort, sm.Config.SocatSSLCertFile)
		if len(sm.Config.SocatSSLCAFile) > 0 {
			socatConOpts += fmt.Sprintf(",cafile=%s", sm.Config.SocatSSLCAFile)
		}
		if sm.Config.SocatSSLSkipVerify {
			socatConOpts += ",verify=0"
		}
	}
	backupCmd := fmt.Sprintf("socat EXEC:\"tar czf - -C %s .\" %s", sm.BackupDir, socatConOpts)
	sm.Logger.Info("Starting backup")
	err := sm.Cmd.CommandRunWithFunc(backupCmd, func(cmd *pipe.State) {
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

func (sm *LVMSeed) Restore() {
	stage := NewSeedStage(Restore, sm.StatusChan)
	cleanupDatadirCmd := fmt.Sprintf("rm -rf %s", path.Join(sm.MySQLDatadir, "auto.cnf"))
	err := sm.Cmd.CommandRunWithFunc(cleanupDatadirCmd, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Removing auto.cnf from MySQL datadir", sm.StatusChan)
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	// change owner of mysql datadir files to mysql:mysql
	chownCmd := fmt.Sprintf("chown -R mysql:mysql %s", sm.MySQLDatadir)
	err = sm.Cmd.CommandRunWithFunc(chownCmd, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Changing owner of mysql datadir", sm.StatusChan)
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	if err := osagent.MySQLStart(sm.MySQLServiceStartCommand, sm.Cmd); err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
	}
	sm.Logger.Info("Restore completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *LVMSeed) GetMetadata() (*SeedMetadata, error) {
	meta := &SeedMetadata{}
	output, err := sm.Cmd.CommandOutput(fmt.Sprintf("cat %s", path.Join(sm.MySQLDatadir, sm.MetadataFileName)))
	if err != nil {
		sm.Logger.WithField("error", err).Info("Unable to read seed metadata")
		return meta, err
	}
	lines := sm.Cmd.OutputLines(output)
	for _, line := range lines {
		if strings.Contains(line, "File:") {
			meta.LogFile = strings.Trim(strings.Split(line, ":")[1], " ")
		}
		if strings.Contains(line, "Position:") {
			meta.LogPos, err = strconv.ParseInt(strings.Trim(strings.Split(line, ":")[1], " "), 10, 64)
			if err != nil {
				sm.Logger.WithField("error", err).Info("Unable to parse seed metadata")
				return meta, err
			}
		}
		if strings.Contains(line, "Executed_Gtid_Set:") {
			meta.GtidExecuted = strings.Trim(strings.SplitAfterN(line, ":", 2)[1], " ")
			break
		}
	}
	return meta, err
}

func (sm *LVMSeed) Cleanup(side Side) {
	stage := NewSeedStage(Cleanup, sm.StatusChan)
	sm.Logger.Info("Starting cleanup")
	if side == Source {
		if err := osagent.Unmount(sm.BackupDir, sm.Cmd); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Cleanup failed")
			return
		}
	}
	sm.Logger.Info("Cleanup completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)

}

func (sm *LVMSeed) isAvailable() bool {
	err := sm.Cmd.CommandRun("lvs --noheading -o lv_name,vg_name,lv_path,snap_percent,time --sort -time")
	if err != nil {
		return false
	}
	return true
}

func (sm *LVMSeed) getSupportedEngines() []mysql.Engine {
	return []mysql.Engine{mysql.ROCKSDB, mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED, mysql.TokuDB}
}

func (sm *LVMSeed) backupToDatadir() bool {
	return true
}
