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
	SudoUser              string           `toml:"sudo-user"`
	BackupDir             string           `toml:"backup-dir"`
	StatusEndpoint        string           `toml:"status-endpoint"`
	StatusBadSeconds      *config.Duration `toml:"status-bad-seconds"`
}

type customCommand struct {
	Cmd string `toml:"cmd"`
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
	Port                 int    `toml:"port"`
	User                 string `toml:"user"`
	Password             string `toml:"password"`
	ServiceStatusCommand string `toml:"service-status-command"`
	ServiceStartCommand  string `toml:"service-start-command"`
	ServiceStopCommand   string `toml:"service-stop-command"`
}

// Config is used to store all configuration parameters
type Config struct {
	Common         commonConfig             `toml:"common"`
	Orchestrator   orchestratorConfig       `toml:"orchestrator"`
	Logging        loggingConfig            `toml:"logging"`
	Mysql          mysqlConfig              `toml:"mysql"`
	MysqlDump      *seed.MysqldumpConfig    `toml:"mysqldump"`
	Mydumper       *seed.MydumperConfig     `toml:"mydumper"`
	Xtrabackup     *seed.XtrabackupConfig   `toml:"xtrabackup"`
	LVM            *seed.LVMConfig          `toml:"lvm"`
	ClonePlugin    *seed.ClonePluginConfig  `toml:"clone_plugin"`
	CustomCommands map[string]customCommand `toml:"custom-commands"`
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
			SudoUser:          "",
			BackupDir:         "",
			StatusEndpoint:    "/api/status",
			StatusBadSeconds: &config.Duration{
				Duration: 300 * time.Second,
			},
		},
		CustomCommands: make(map[string]customCommand),
		Orchestrator: orchestratorConfig{
			URL:        "",
			AgentsPort: 3001,
		},
		Logging: loggingConfig{
			File:  "/var/log/orchestrator-agent.log",
			Level: "Info",
		},
		Mysql: mysqlConfig{
			Port:                 3306,
			User:                 "",
			Password:             "",
			ServiceStatusCommand: "systemctl check mysqld",
			ServiceStartCommand:  "systemctl start mysqld",
			ServiceStopCommand:   "systemctl stop mysqld",
		},
		MysqlDump: &seed.MysqldumpConfig{
			Enabled:                 true,
			MysqldumpAdditionalOpts: []string{"--single-transaction", "--quick", "--routines", "--events", "--triggers", "--hex-blob"},
		},
		Mydumper: &seed.MydumperConfig{
			Enabled:                false,
			MydumperAdditionalOpts: []string{"--routines", "--events", "--triggers"},
			MyloaderAdditionalOpts: []string{},
		},
		Xtrabackup: &seed.XtrabackupConfig{
			Enabled:                  false,
			XtrabackupAdditionalOpts: []string{},
			SocatUseSSL:              false,
			SocatSSLCertFile:         "",
			SocatSSLCAFile:           "",
			SocatSSLSkipVerify:       false,
		},
		LVM: &seed.LVMConfig{
			Enabled:                            false,
			CreateNewSnapshotForSeed:           false,
			CreateSnapshotCommand:              "",
			AvailableLocalSnapshotHostsCommand: "",
			AvailableSnapshotHostsCommand:      "",
			SnapshotVolumesFilter:              "",
			SocatUseSSL:                        false,
			SocatSSLCertFile:                   "",
			SocatSSLCAFile:                     "",
			SocatSSLSkipVerify:                 false,
		},
		ClonePlugin: &seed.ClonePluginConfig{
			Enabled:                  false,
			CloneAutotuneConcurrency: true,
			CloneBufferSize:          4194304,
			CloneDDLTimeout:          300,
			CloneEnableCompression:   false,
			CloneMaxConcurrency:      16,
			CloneMaxDataBandwidth:    0,
			CloneMaxNetworkBandwidth: 0,
			CloneSSLCa:               "",
			CloneSSLCert:             "",
			CloneSSLKey:              "",
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
