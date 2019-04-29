package main

import (
	"fmt"
)

type mysqldump struct {
	engines        []string
	streaming      bool
	physicalBackup bool
}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
)

func (m mysqldump) Backup() {
	fmt.Println("This is lvm backup")
}

func (m mysqldump) Restore() {
	fmt.Println("This is lvm restore")
}

func (m mysqldump) GetMetadata() {
	fmt.Println("This is lvm metadata")
}

func (m mysqldump) SupportedEngines() []string {
	return m.engines
}

func (m mysqldump) IsAvailiable() bool {
	return true
}

func (m mysqldump) SupportStreaming() bool {
	return m.streaming
}

func (m mysqldump) SupportPhysicalBackup() bool {
	return m.physicalBackup
}

var BackupPlugin = mysqldump{
	engines:        []string{"InnoDB", "MyISAM", "ROCKSDB", "TokuDB"},
	streaming:      true,
	physicalBackup: false,
}

func main() {

}
