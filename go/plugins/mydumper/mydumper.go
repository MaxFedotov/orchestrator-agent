package main

import (
	"fmt"
)

type mydumper struct {
	engines        []string
	streaming      bool
	physicalBackup bool
}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
)

func (m mydumper) Backup() {
	fmt.Println("This is lvm backup")
}

func (m mydumper) Restore() {
	fmt.Println("This is lvm restore")
}

func (m mydumper) GetMetadata() {
	fmt.Println("This is lvm metadata")
}

func (m mydumper) SupportedEngines() []string {
	return m.engines
}

func (m mydumper) IsAvailiable() bool {
	return true
}

func (m mydumper) SupportStreaming() bool {
	return m.streaming
}

func (m mydumper) SupportPhysicalBackup() bool {
	return m.physicalBackup
}

var BackupPlugin = mydumper{
	engines:        []string{"InnoDB", "MyISAM", "ROCKSDB", "TokuDB"},
	streaming:      true,
	physicalBackup: false,
}

func main() {

}
