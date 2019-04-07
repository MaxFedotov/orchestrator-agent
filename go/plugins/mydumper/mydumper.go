package main

import (
	"fmt"
)

type mydumper struct{}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
)

func (m mydumper) Backup() {
	fmt.Println("This is mydumper backup")
}

func (m mydumper) Restore() {
	fmt.Println("This is mydumper restore")
}

func (m mydumper) GetMetadata() {
	fmt.Println("This is mydumper metadata")
}

func (m mydumper) SupportedEngines() []string {
	return []string{"InnoDB", "MyISAM", "ROCKSDB", "TokuDB"}
}

func (m mydumper) IsAvailiable() bool {
	return false
}

var BackupPlugin = mydumper{}

func main() {

}
