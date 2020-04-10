package seed

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/github/orchestrator-agent/go/helper/mysql"
	"github.com/github/orchestrator-agent/go/osagent"
	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
)

var defaultXtrabackupOpts = map[string]bool{
	"--host":       true,
	"-h":           true,
	"--user":       true,
	"-u":           true,
	"--port":       true,
	"-P":           true,
	"--password":   true,
	"-p":           true,
	"--target-dir": true,
	"--decompress": true,
	"--backup":     true,
	"--stream":     true,
	"--prepare":    true,
}

type XtrabackupSeed struct {
	*Base
	*MethodOpts
	Config           *XtrabackupConfig
	Logger           *log.Entry
	MetadataFileName string
}

type XtrabackupConfig struct {
	Enabled                  bool     `toml:"enabled"`
	XtrabackupAdditionalOpts []string `toml:"xtrabackup-addtional-opts"`
	SocatUseSSL              bool     `toml:"socat-use-ssl"`
	SocatSSLCertFile         string   `toml:"socat-ssl-cert-file"`
	SocatSSLCAFile           string   `toml:"socat-ssl-cat-file"`
	SocatSSLSkipVerify       bool     `toml:"socat-ssl-skip-verify"`
}

func (sm *XtrabackupSeed) Prepare(side Side) {
	stage := NewSeedStage(Prepare, sm.StatusChan, sm.Hostname)
	sm.Logger.Info("Starting prepare")
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
		createLockFileCmd := fmt.Sprintf("touch %s/orchestrator-agent.lock", sm.MySQLDatadir)
		err = sm.Cmd.CommandRunWithFunc(createLockFileCmd, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Creating lock file", sm.StatusChan)
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
			recieveCmd := fmt.Sprintf("socat -u %s EXEC:\"xbstream -x - -C %s\"", socatConOpts, sm.MySQLDatadir)
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

func (sm *XtrabackupSeed) Backup(seedHost string, mysqlPort int) {
	stage := NewSeedStage(Backup, sm.StatusChan, sm.Hostname)
	var addtionalOpts []string
	for _, opt := range sm.Config.XtrabackupAdditionalOpts {
		if defaultXtrabackupOpts[strings.Split(opt, "=")[0]] {
			sm.Logger.WithField("XtrabackupOption", opt).Error("Will skip xtrabackup option, as it is already used by default")
		} else {
			addtionalOpts = append(addtionalOpts, opt)
		}
	}
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
	backupCmd := fmt.Sprintf("socat EXEC:\"xtrabackup --backup --target-dir=./ --user=%s --password=%s --port=%d --host=127.0.0.1 --stream=xbstream %s\" %s", sm.User, sm.Password, mysqlPort, strings.Join(addtionalOpts, " "), socatConOpts)
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

func (sm *XtrabackupSeed) Restore() {
	stage := NewSeedStage(Restore, sm.StatusChan, sm.Hostname)
	var decompress bool
	var parallel = 1
	var err error
	sm.Logger.Info("Starting restore")
	for _, opt := range sm.Config.XtrabackupAdditionalOpts {
		switch strings.Split(opt, "=")[0] {
		case "--compress":
			decompress = true
		case "--parallel":
			parallel, err = strconv.Atoi(strings.Split(opt, "=")[1])
			if err != nil {
				parallel = 1
				sm.Logger.WithField("error", err).Info("Unable to parse xtrabackup parallel option")
			}
		}
	}
	if decompress {
		decompressCmd := fmt.Sprintf("xtrabackup --decompress --parallel=%d --target-dir=%s", parallel, sm.MySQLDatadir)
		err := sm.Cmd.CommandRunWithFunc(decompressCmd, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Decompressing backup", sm.StatusChan)
		})
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Restore failed")
			return
		}
		removeCompressedFilesCmd := fmt.Sprintf("find %s -type f -regex ^.*.qp$ -delete", sm.MySQLDatadir)
		err = sm.Cmd.CommandRunWithFunc(removeCompressedFilesCmd, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Removing compressed backup files", sm.StatusChan)
		})
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Restore failed")
			return
		}
	}
	// we need this in order to prevent possible xtrabackup error "Log file ./ib_logfile1 is of different size than other log files bytes!"
	removeLogFilesCmd := fmt.Sprintf("find %s -type f -regex ^.*ib_logfile[0-9]$ -delete", sm.MySQLDatadir)
	err = sm.Cmd.CommandRunWithFunc(removeLogFilesCmd, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Removing ib_logfile files", sm.StatusChan)
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	prepareCmd := fmt.Sprintf("xtrabackup --prepare --target-dir=%s", sm.MySQLDatadir)
	err = sm.Cmd.CommandRunWithFunc(prepareCmd, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Running xtrabackup prepare", sm.StatusChan)
	})
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	// remove ib_logfile created by xtrabackup
	err = sm.Cmd.CommandRunWithFunc(removeLogFilesCmd, func(cmd *pipe.State) {
		stage.UpdateSeedStatus(Running, cmd, "Removing ib_logfile files", sm.StatusChan)
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

func (sm *XtrabackupSeed) GetMetadata() (*SeedMetadata, error) {
	meta := &SeedMetadata{}
	output, err := sm.Cmd.CommandOutput(fmt.Sprintf("cat %s", path.Join(sm.MySQLDatadir, sm.MetadataFileName)))
	if err != nil {
		sm.Logger.WithField("error", err).Info("Unable to read seed metadata")
		return meta, err
	}
	lines := sm.Cmd.OutputLines(output)
	for idx, line := range lines {
		if idx == 0 {
			tokens := strings.Split(line, "\t")
			meta.LogFile = tokens[0]
			meta.LogPos, err = strconv.ParseInt(strings.Trim(tokens[1], "\n"), 10, 64)
			if err != nil {
				sm.Logger.WithField("error", err).Info("Unable to parse seed metadata")
				return meta, err
			}
			if len(tokens) > 2 {
				meta.GtidExecuted = strings.Trim(tokens[2], "\n")
			}

		} else {
			meta.GtidExecuted += strings.Trim(line, "\n")
		}
	}
	return meta, err
}

func (sm *XtrabackupSeed) Cleanup(side Side) {
	stage := NewSeedStage(Cleanup, sm.StatusChan, sm.Hostname)
	sm.Logger.Info("Starting cleanup")
	if side == Target {
		removeLockFileCmd := fmt.Sprintf("rm -rf %s/orchestrator-agent.lock", sm.MySQLDatadir)
		err := sm.Cmd.CommandRunWithFunc(removeLockFileCmd, func(cmd *pipe.State) {
			stage.UpdateSeedStatus(Running, cmd, "Creating lock file", sm.StatusChan)
		})
		if err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
	}
	sm.Logger.Info("Cleanup completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *XtrabackupSeed) isAvailable() bool {
	err := sm.Cmd.CommandRun("xtrabackup --version")
	if err != nil {
		return false
	}
	return true
}

func (sm *XtrabackupSeed) getSupportedEngines() []mysql.Engine {
	output, _ := sm.Cmd.CommandCombinedOutput("xtrabackup --version")
	xtrabackupVersionString := string(output)
	re := regexp.MustCompile(`version\s*(\d+)`)
	res := re.FindStringSubmatch(xtrabackupVersionString)[1]
	xtrabackupVersion, _ := strconv.Atoi(res)
	if xtrabackupVersion < 8 {
		return []mysql.Engine{mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED}
	}
	return []mysql.Engine{mysql.ROCKSDB, mysql.MRG_MYISAM, mysql.CSV, mysql.BLACKHOLE, mysql.InnoDB, mysql.MEMORY, mysql.ARCHIVE, mysql.MyISAM, mysql.FEDERATED}
}

func (sm *XtrabackupSeed) backupToDatadir() bool {
	return true
}
