package seed

import (
	"bytes"
	"fmt"

	"github.com/github/orchestrator-agent/go/helper/mysql"
	log "github.com/sirupsen/logrus"
)

type Side int

const (
	Target Side = iota
	Source
)

func (s Side) String() string {
	return [...]string{"Target", "Source"}[s]
}

var ToSide = map[string]Side{
	"Target": Target,
	"Source": Source,
}

// MarshalJSON marshals the enum as a quoted json string
func (s Side) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(s.String())
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

type Method int

const (
	ClonePlugin Method = iota
	LVM
	Mydumper
	Mysqldump
	Xtrabackup
)

func (m Method) String() string {
	return [...]string{"ClonePlugin", "LVM", "Mydumper", "Mysqldump", "Xtrabackup"}[m]
}

// MarshalText marshals the enum as a json key
func (m Method) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

var ToMethod = map[string]Method{
	"ClonePlugin": ClonePlugin,
	"LVM":         LVM,
	"Mydumper":    Mydumper,
	"Mysqldump":   Mysqldump,
	"Xtrabackup":  Xtrabackup,
}

type Plugin interface {
	Prepare(side Side)
	Backup(seedHost string, mysqlPort int)
	Restore()
	GetMetadata() (*BackupMetadata, error)
	Cleanup(side Side)
	isAvailable() bool
	getSupportedEngines() []mysql.Engine
	backupToDatadir() bool
}

type Base struct {
	MySQLClient   *mysql.MySQLClient
	MySQLPort     int
	SeedUser      string
	SeedPassword  string
	ExecWithSudo  bool
	SeedPort      int
	UseSSL        bool
	SSLSkipVerify bool
	SSLCertFile   string
	SSLCAFile     string
	BackupDir     string
	StatusChan    chan *StageStatus
}

type MethodOpts struct {
	BackupSide       Side
	SupportedEngines []mysql.Engine
	BackupToDatadir  bool
}

type BackupMetadata struct {
	LogFile      string
	LogPos       int64
	GtidExecuted string
}

// New creates seed plugin
func New(seedMethod Method, baseConfig *Base, logger *log.Entry, seedMethodConfig interface{}) (Plugin, *MethodOpts, error) {
	if seedMethod == LVM {
		if conf, ok := seedMethodConfig.(*LVMConfig); ok {
			sm := &LVMSeed{
				Base:       baseConfig,
				MethodOpts: &MethodOpts{},
				Config:     conf,
				Logger:     logger,
			}
			if sm.isAvailable() {
				sm.MethodOpts.BackupSide = Source
				sm.MethodOpts.SupportedEngines = sm.getSupportedEngines()
				sm.MethodOpts.BackupToDatadir = sm.backupToDatadir()
				return sm, sm.MethodOpts, nil
			}
			return nil, nil, fmt.Errorf("LVM seed method unavailable")
		}
		return nil, nil, fmt.Errorf("unable to parse LVM config")
	}
	if seedMethod == Xtrabackup {
		if conf, ok := seedMethodConfig.(*XtrabackupConfig); ok {
			sm := &XtrabackupSeed{
				Base:       baseConfig,
				MethodOpts: &MethodOpts{},
				Config:     conf,
				Logger:     logger,
			}
			if sm.isAvailable() {
				sm.MethodOpts.BackupSide = Source
				sm.MethodOpts.SupportedEngines = sm.getSupportedEngines()
				sm.MethodOpts.BackupToDatadir = sm.backupToDatadir()
				return sm, sm.MethodOpts, nil
			}
			return nil, nil, fmt.Errorf("Xtrabackup seed method unavailable")
		}
		return nil, nil, fmt.Errorf("unable to parse Xtrabackup config")
	}
	if seedMethod == ClonePlugin {
		if conf, ok := seedMethodConfig.(*ClonePluginConfig); ok {
			sm := &ClonePluginSeed{
				Base:       baseConfig,
				MethodOpts: &MethodOpts{},
				Config:     conf,
				Logger:     logger,
			}
			if sm.isAvailable() {
				sm.MethodOpts.BackupSide = Target
				sm.MethodOpts.SupportedEngines = sm.getSupportedEngines()
				sm.MethodOpts.BackupToDatadir = sm.backupToDatadir()
				return sm, sm.MethodOpts, nil
			}
			return nil, nil, fmt.Errorf("Clone plugin seed method unavailable")
		}
		return nil, nil, fmt.Errorf("unable to parse Clone plugin config")
	}
	if seedMethod == Mydumper {
		if conf, ok := seedMethodConfig.(*MydumperConfig); ok {
			sm := &MydumperSeed{
				Base:       baseConfig,
				MethodOpts: &MethodOpts{},
				Config:     conf,
				Logger:     logger,
			}
			if sm.isAvailable() {
				sm.MethodOpts.BackupSide = Target
				sm.MethodOpts.SupportedEngines = sm.getSupportedEngines()
				sm.MethodOpts.BackupToDatadir = sm.backupToDatadir()
				return sm, sm.MethodOpts, nil
			}
			return nil, nil, fmt.Errorf("Mydumper seed method unavailable")
		}
		return nil, nil, fmt.Errorf("unable to parse Mydumper config")
	}
	if seedMethod == Mysqldump {
		if conf, ok := seedMethodConfig.(*MysqldumpConfig); ok {
			sm := &MysqldumpSeed{
				Base:       baseConfig,
				MethodOpts: &MethodOpts{},
				Config:     conf,
				Logger:     logger,
			}
			if sm.isAvailable() {
				sm.MethodOpts.BackupSide = Target
				sm.MethodOpts.SupportedEngines = sm.getSupportedEngines()
				sm.MethodOpts.BackupToDatadir = sm.backupToDatadir()
				return sm, sm.MethodOpts, nil
			}
			return nil, nil, fmt.Errorf("Mysqldump seed method unavailable")
		}
		return nil, nil, fmt.Errorf("unable to parse Mysqldump config")
	}
	return nil, nil, fmt.Errorf("unknown seed method")
}
