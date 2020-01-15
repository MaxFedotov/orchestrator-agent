package seed

import (
	"context"

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

func (sm *ClonePluginSeed) Prepare(ctx context.Context, side Side) error {
	sm.Logger.Info("This is clone plugin prepare")
	return nil
}

func (sm *ClonePluginSeed) Backup(ctx context.Context) error {
	sm.Logger.Info("This is clone plugin backup")
	return nil
}

func (sm *ClonePluginSeed) Restore(ctx context.Context) error {
	sm.Logger.Info("This is clone plugin restore")
	return nil
}

func (sm *ClonePluginSeed) GetMetadata(ctx context.Context) (*BackupMetadata, error) {
	sm.Logger.Info("This is clone plugin metadata")
	return &BackupMetadata{}, nil
}

func (sm *ClonePluginSeed) Cleanup(ctx context.Context, side Side) error {
	sm.Logger.Info("This is clone plugin cleanup")
	return nil
}

func (sm *ClonePluginSeed) IsAvailable() bool {
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
