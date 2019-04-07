package main

import (
	"fmt"
)

type mysqldump struct{}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
)

func (m mysqldump) Backup() {
	fmt.Println("This is mysqldump backup")
}

func (m mysqldump) Restore() {
	fmt.Println("This is mysqldump restore")
}

func (m mysqldump) GetMetadata() {
	fmt.Println("This is mysqldump metadata")
}

func (m mysqldump) SupportedEngines() []string {
	return []string{"InnoDB", "MyISAM", "ROCKSDB", "TokuDB"}
}

func (m mysqldump) IsAvailiable() bool {
	return true
}

var BackupPlugin = mysqldump{}

func main() {

}
