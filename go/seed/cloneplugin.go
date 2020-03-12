package seed

import (
	"fmt"
	"time"

	"github.com/cenkalti/backoff"
	"github.com/github/orchestrator-agent/go/helper/cmd"
	"github.com/github/orchestrator-agent/go/helper/mysql"
	"github.com/github/orchestrator-agent/go/osagent"
	"github.com/openark/golib/sqlutils"
	log "github.com/sirupsen/logrus"
	"gopkg.in/pipe.v2"
)

type ClonePluginSeed struct {
	*Base
	*MethodOpts
	Config *ClonePluginConfig
	Logger *log.Entry
}

type ClonePluginConfig struct {
	Enabled                  bool   `toml:"enabled"`
	CloneAutotuneConcurrency bool   `toml:"clone-autotune-concurrency"`
	CloneBufferSize          int64  `toml:"clone-buffer-size"`
	CloneDDLTimeout          int64  `toml:"clone-ddl-timeout"`
	CloneEnableCompression   bool   `toml:"clone-enable-compression"`
	CloneMaxConcurrency      int    `toml:"clone-max-concurrency"`
	CloneMaxDataBandwidth    int    `toml:"clone-max-data-bandwidth"`
	CloneMaxNetworkBandwidth int    `toml:"clone-max-network-bandwidth"`
	CloneSSLCa               string `toml:"clone-ssl-ca"`
	CloneSSLCert             string `toml:"clone-ssl-cert"`
	CloneSSLKey              string `toml:"clone-ssl-key"`
}

func (sm *ClonePluginSeed) Prepare(side Side) {
	stage := NewSeedStage(Prepare, sm.StatusChan)
	sm.Logger.Info("Starting prepare")
	if side == Target {
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_autotune_concurrency=%t", sm.Config.CloneAutotuneConcurrency)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_buffer_size=%d", sm.Config.CloneBufferSize)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_ddl_timeout=%d", sm.Config.CloneDDLTimeout)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_enable_compression=%t", sm.Config.CloneEnableCompression)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_max_concurrency=%d", sm.Config.CloneMaxConcurrency)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_max_data_bandwidth=%d", sm.Config.CloneMaxNetworkBandwidth)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_ssl_ca='%s'", sm.Config.CloneSSLCa)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_ssl_cert='%s'", sm.Config.CloneSSLCert)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
		if err := mysql.Exec(sm.MySQLClient.Conn, fmt.Sprintf("SET GLOBAL clone_ssl_key='%s'", sm.Config.CloneSSLKey)); err != nil {
			stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
			sm.Logger.WithField("error", err).Info("Prepare failed")
			return
		}
	}
	sm.Logger.Info("Prepare completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *ClonePluginSeed) Backup(seedHost string, mysqlPort int) {
	stage := NewSeedStage(Backup, sm.StatusChan)
	sm.Logger.Info("Starting backup")
	donorListCmd := fmt.Sprintf("SET GLOBAL clone_valid_donor_list ='%s:%d'", seedHost, mysqlPort)
	if err := mysql.Exec(sm.MySQLClient.Conn, donorListCmd); err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Backup failed")
		return
	}
	cloneCmd := fmt.Sprintf("mysql --user=%s --password=%s --host=127.0.0.1 --port=%d -BNe \"CLONE INSTANCE FROM %s@%s:%d identified by '%s';\"", sm.SeedUser, sm.SeedPassword, sm.MySQLPort, sm.SeedUser, seedHost, mysqlPort, sm.SeedPassword)
	err := cmd.CommandRunWithFunc(cloneCmd, sm.ExecWithSudo, func(cmd *pipe.State) {
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

func (sm *ClonePluginSeed) isMySQLRunning() error {
	running, err := osagent.MySQLRunning(sm.ExecWithSudo)
	if running == false {
		err = fmt.Errorf("MySQL not running")
	}
	return err
}

func (sm *ClonePluginSeed) Restore() {
	stage := NewSeedStage(Restore, sm.StatusChan)
	sm.Logger.Info("Starting restore")
	b := backoff.NewExponentialBackOff()
	b.MaxElapsedTime = 15 * time.Minute
	b.InitialInterval = 1 * time.Second
	checkMySQLStatus := func() error {
		return sm.isMySQLRunning()
	}
	notify := func(err error, t time.Duration) {
		stage.UpdateSeedStatus(Running, nil, "Running restore. Waiting for MySQL to start", sm.StatusChan)
	}
	err := backoff.RetryNotify(checkMySQLStatus, b, notify)
	if err != nil {
		stage.UpdateSeedStatus(Error, nil, err.Error(), sm.StatusChan)
		sm.Logger.WithField("error", err).Info("Restore failed")
		return
	}
	sm.Logger.Info("Restore completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *ClonePluginSeed) GetMetadata() (*SeedMetadata, error) {
	meta := &SeedMetadata{}
	metdadataQuery := `SELECT BINLOG_FILE, BINLOG_POSITION, GTID_EXECUTED FROM performance_schema.clone_status;`
	err := mysql.QueryData(sm.MySQLClient.Conn, metdadataQuery, nil, func(m sqlutils.RowMap) error {
		meta.LogFile = m.GetString("BINLOG_FILE")
		meta.LogPos = m.GetInt64("BINLOG_POSITION")
		meta.GtidExecuted = m.GetString("GTID_EXECUTED")
		return nil
	})
	return meta, err
}

func (sm *ClonePluginSeed) Cleanup(side Side) {
	stage := NewSeedStage(Cleanup, sm.StatusChan)
	sm.Logger.Info("Starting cleanup")
	sm.Logger.Info("Cleanup completed")
	stage.UpdateSeedStatus(Completed, nil, "Stage completed", sm.StatusChan)
}

func (sm *ClonePluginSeed) isAvailable() bool {
	installed, err := getPluginStatus(sm.MySQLClient, "clone")
	if err != nil {
		return false
	}
	return installed
}

// getPluginStatus returns if plugin is installed
func getPluginStatus(m *mysql.MySQLClient, pluginName string) (installed bool, err error) {
	var pluginStatus string
	query := `SELECT PLUGIN_STATUS FROM INFORMATION_SCHEMA.PLUGINS WHERE PLUGIN_NAME = ?`
	err = mysql.QueryData(m.Conn, query, sqlutils.Args(pluginName), func(m sqlutils.RowMap) error {
		pluginStatus = m.GetString("PLUGIN_STATUS")
		return nil
	})
	if pluginStatus == "ACTIVE" {
		return true, err
	}
	return false, err
}

func (sm *ClonePluginSeed) getSupportedEngines() []mysql.Engine {
	return []mysql.Engine{mysql.InnoDB}
}

func (sm *ClonePluginSeed) backupToDatadir() bool {
	return true
}
