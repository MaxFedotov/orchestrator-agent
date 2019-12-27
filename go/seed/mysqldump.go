package seed

import (
	"context"
	"fmt"

	log "github.com/sirupsen/logrus"
)

type Mysqldump struct {
	DatabaseSelection bool
	SeedSide          seedSide
	Logger            *log.Entry
	ExecBackup        string //do we need it?
	ExecRestore       string // do we need it
}

const (
	mysqlbackupFileName           = "backup.sql"
	mysqlbackupCompressedFileName = "backup.sql.gz"
)

func (m Mysqldump) Prepare(ctx context.Context) error {
	fmt.Println("This is mysqldump prepare")
	return nil
}

func (m Mysqldump) Backup(ctx context.Context) error {
	fmt.Println("This is mysqldump backup")
	return nil
}

func (m Mysqldump) Restore(ctx context.Context) error {
	fmt.Println("This is mysqldump restore")
	return nil
}

func (m Mysqldump) GetMetadata(ctx context.Context) (BackupMetadata, error) {
	fmt.Println("This is mysqldump metadata")
	return BackupMetadata{}, nil
}

func (m Mysqldump) Cleanup(ctx context.Context) error {
	fmt.Println("This is mysqldump cleanup")
	return nil
}

func (m Mysqldump) IsAvailiable() bool {
	return true
}
