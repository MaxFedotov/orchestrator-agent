package seed

import (
	"bytes"
	"context"
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

type Seed interface {
	Prepare(ctx context.Context, side Side) error
	Backup(ctx context.Context) error
	Restore(ctx context.Context) error
	GetMetadata(ctx context.Context) (*BackupMetadata, error)
	Cleanup(ctx context.Context, side Side) error
	IsAvailable() bool
}

type Base struct {
	MySQLClient      *mysql.MySQLClient
	ExecWithSudo     bool
	SeedPort         int
	UseSSL           bool
	SSLSkipVerify    bool
	SSLCertFile      string
	SSLCAFile        string
	BackupDir        string
	BackupOldDatadir bool
}

type MethodOpts struct {
	DatabaseSelection bool
	BackupSide        Side
}

type BackupMetadata struct {
	LogFile      string
	LogPos       int64
	GtidExecuted string
}

// New creates seed method
func New(seedMethod Method, baseConfig *Base, methodOpts *MethodOpts, logger *log.Entry, seedMethodConfig interface{}) (Seed, error) {
	if seedMethod == LVM {
		if conf, ok := seedMethodConfig.(*LVMConfig); ok {
			sm := &LVMSeed{
				Base:       baseConfig,
				MethodOpts: methodOpts,
				Config:     conf,
				Logger:     logger,
			}
			if sm.IsAvailable() {
				return sm, nil
			}
			return nil, fmt.Errorf("LVM seed method unavailable")
		}
		return nil, fmt.Errorf("unable to parse LVM config")
	}
	if seedMethod == Xtrabackup {
		if conf, ok := seedMethodConfig.(*XtrabackupConfig); ok {
			sm := &XtrabackupSeed{
				Base:       baseConfig,
				MethodOpts: methodOpts,
				Config:     conf,
				Logger:     logger,
			}
			if sm.IsAvailable() {
				return sm, nil
			}
			return nil, fmt.Errorf("Xtrabackup seed method unavailable")
		}
		return nil, fmt.Errorf("unable to parse Xtrabackup config")
	}
	if seedMethod == ClonePlugin {
		if conf, ok := seedMethodConfig.(*ClonePluginConfig); ok {
			sm := &ClonePluginSeed{
				Base:       baseConfig,
				MethodOpts: methodOpts,
				Config:     conf,
				Logger:     logger,
			}
			if sm.IsAvailable() {
				return sm, nil
			}
			return nil, fmt.Errorf("Clone plugin seed method unavailable")
		}
		return nil, fmt.Errorf("unable to parse Clone plugin config")
	}
	if seedMethod == Mydumper {
		if conf, ok := seedMethodConfig.(*MydumperConfig); ok {
			sm := &MydumperSeed{
				Base:       baseConfig,
				MethodOpts: methodOpts,
				Config:     conf,
				Logger:     logger,
			}
			if sm.IsAvailable() {
				return sm, nil
			}
			return nil, fmt.Errorf("Mydumper seed method unavailable")
		}
		return nil, fmt.Errorf("unable to parse Mydumper config")
	}
	if seedMethod == Mysqldump {
		if conf, ok := seedMethodConfig.(*MysqldumpConfig); ok {
			sm := &MysqldumpSeed{
				Base:       baseConfig,
				MethodOpts: methodOpts,
				Config:     conf,
				Logger:     logger,
			}
			if sm.IsAvailable() {
				return sm, nil
			}
			return nil, fmt.Errorf("Mysqldump seed method unavailable")
		}
		return nil, fmt.Errorf("unable to parse Mysqldump config")
	}
	return nil, fmt.Errorf("unknown seed method")
}
