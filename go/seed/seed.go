package seed

import (
	"context"
)

type seedSide int

const (
	Target seedSide = iota
	Source
)

type SeedMethod interface {
	Prepare(ctx context.Context) error
	Backup(ctx context.Context) error
	Restore(ctx context.Context) error
	GetMetadata(ctx context.Context) (BackupMetadata, error)
	Cleanup(ctx context.Context) error
	IsAvaliable() bool
}

type BackupMetadata struct {
	LogFile        string
	LogPos         int64
	GtidExecuted   string
	MasterUser     string // This is optional field. If it is empty, will be read from configuration file
	MasterPassword string // This is optional field. If it is empty, will be read from configuration file
}

type BackupMethod struct{}

func (b *BackupMethod) Prepare(ctx context.Context) error { return nil }
func (b *BackupMethod) Backup(ctx context.Context) error  { return nil }
func (b *BackupMethod) Restore(ctx context.Context) error { return nil }

// other methods goes here, then for each seed method override this basic methods

// NewBackupMethod func where we
