package seed

import (
	"github.com/github/orchestrator-agent/go/helper/mysql"
	"github.com/openark/golib/sqlutils"
	log "github.com/sirupsen/logrus"
)

type ClonePluginSeed struct {
	*Base
	*MethodOpts
	Config *ClonePluginConfig
	Logger *log.Entry
}

type ClonePluginConfig struct {
	Enabled bool `toml:"enabled"`
}

func (sm *ClonePluginSeed) Prepare(side Side) {
	sm.Logger.Info("This is clone plugin prepare")
}

func (sm *ClonePluginSeed) Backup(seedHost string, mysqlPort int) {
	sm.Logger.Info("This is clone plugin backup")
}

func (sm *ClonePluginSeed) Restore() {
	sm.Logger.Info("This is clone plugin restore")
}

func (sm *ClonePluginSeed) GetMetadata() (*BackupMetadata, error) {
	sm.Logger.Info("This is clone plugin metadata")
	return &BackupMetadata{}, nil
}

func (sm *ClonePluginSeed) Cleanup(side Side) {
	sm.Logger.Info("This is clone plugin cleanup")
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
