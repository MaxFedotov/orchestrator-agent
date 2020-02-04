package agent

import (
	"time"

	"github.com/BurntSushi/toml"
	"github.com/github/orchestrator-agent/go/helper/config"
	"github.com/github/orchestrator-agent/go/seed"
)

type commonConfig struct {
	Port                  int              `toml:"port"`
	SeedPort              int              `toml:"seed-port"`
	PollInterval          *config.Duration `toml:"poll-interval"`
	ResubmitAgentInterval *config.Duration `toml:"resubmit-agent-interval"`
	HTTPAuthUser          string           `toml:"http-auth-user"`
	HTTPAuthPassword      string           `toml:"http-auth-password"`
	HTTPTimeout           *config.Duration `toml:"http-timeout"`
	UseSSL                bool             `toml:"use-ssl"`
	UseMutualTLS          bool             `toml:"use-mutual-tls"`
	SSLSkipVerify         bool             `toml:"ssl-skip-verify"`
	SSLCertFile           string           `toml:"ssl-cert-file"`
	SSLPrivateKeyFile     string           `toml:"ssl-private-key-file"`
	SSLCAFile             string           `toml:"ssl-ca-file"`
	SSLValidOUs           []string         `toml:"ssl-valid-ous"`
	StatusOUVerify        bool             `toml:"status-ou-verify"`
	TokenHintFile         string           `toml:"token-hint-file"`
	TokenHTTPHeader       string           `toml:"token-http-header"`
	ExecWithSudo          bool             `toml:"exec-with-sudo"`
	BackupDir             string           `toml:"backup-dir"`
	PostSeedCommand       string           `toml:"post-seed-command"`
	StatusEndpoint        string           `toml:"status-endpoint"`
}

type orchestratorConfig struct {
	URL        string `toml:"url"`
	AgentsPort int    `toml:"agents-port"`
}

type loggingConfig struct {
	File  string `toml:"file"`
	Level string `toml:"level"`
}

type mysqlConfig struct {
	Port                int    `toml:"port"`
	SeedUser            string `toml:"seed-user"`
	SeedPassword        string `toml:"seed-password"`
	ReplicationUser     string `toml:"replication-user"`
	ReplicationPassword string `toml:"replication-password"`
}

// Config is used to store all configuration parameters
type Config struct {
	Common       commonConfig            `toml:"common"`
	Orchestrator orchestratorConfig      `toml:"orchestrator"`
	Logging      loggingConfig           `toml:"logging"`
	Mysql        mysqlConfig             `toml:"mysql"`
	MysqlDump    *seed.MysqldumpConfig   `toml:"mysqldump"`
	Mydumper     *seed.MydumperConfig    `toml:"mydumper"`
	Xtrabackup   *seed.XtrabackupConfig  `toml:"xtrabackup"`
	LVM          *seed.LVMConfig         `toml:"lvm"`
	ClonePlugin  *seed.ClonePluginConfig `toml:"clone_plugin"`
}

// NewConfig sets default values for configuration parameters
func NewConfig() *Config {
	config := &Config{
		Common: commonConfig{
			Port:     3002,
			SeedPort: 21234,
			PollInterval: &config.Duration{
				Duration: time.Minute,
			},
			ResubmitAgentInterval: &config.Duration{
				Duration: time.Hour,
			},
			HTTPAuthUser:     "",
			HTTPAuthPassword: "",
			HTTPTimeout: &config.Duration{
				Duration: 10 * time.Second,
			},
			UseSSL:            false,
			UseMutualTLS:      false,
			SSLSkipVerify:     false,
			SSLCertFile:       "",
			SSLPrivateKeyFile: "",
			SSLCAFile:         "",
			SSLValidOUs:       []string{},
			StatusOUVerify:    false,
			TokenHintFile:     "",
			TokenHTTPHeader:   "",
			ExecWithSudo:      false,
			BackupDir:         "",
			PostSeedCommand:   "",
			StatusEndpoint:    "/api/status",
		},
		Orchestrator: orchestratorConfig{
			URL:        "",
			AgentsPort: 3001,
		},
		Logging: loggingConfig{
			File:  "/var/log/orchestrator-agent.log",
			Level: "Info",
		},
		Mysql: mysqlConfig{
			Port:                3306,
			SeedUser:            "",
			SeedPassword:        "",
			ReplicationUser:     "",
			ReplicationPassword: "",
		},
		MysqlDump: &seed.MysqldumpConfig{
			Enabled:  true,
			Compress: true,
		},
		Mydumper: &seed.MydumperConfig{
			Enabled:         false,
			ParallelThreads: 1,
			RowsChunkSize:   0,
			Compress:        false,
		},
		Xtrabackup: &seed.XtrabackupConfig{
			Enabled:         false,
			ParallelThreads: 1,
			Compress:        false,
		},
		LVM: &seed.LVMConfig{
			Enabled:                            false,
			CreateSnapshotCommand:              "",
			AvailableLocalSnapshotHostsCommand: "",
			AvailableSnapshotHostsCommand:      "",
			SnapshotVolumesFilter:              "",
			SnapshotMountPoint:                 "",
		},
		ClonePlugin: &seed.ClonePluginConfig{
			Enabled: false,
		},
	}
	return config
}

//ReadConfig reads TOML configuration file
func ReadConfig(filename string) (*Config, error) {
	config := NewConfig()
	if _, err := toml.DecodeFile(filename, &config); err != nil {
		return nil, err
	}
	return config, nil
}
