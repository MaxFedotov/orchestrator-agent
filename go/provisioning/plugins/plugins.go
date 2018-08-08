package plugins

type BinlogCoordinates struct {
	LogFile string
	LogPos  int64
	Type    BinlogType
}

type BackupMetadata struct {
	BinlogCoordinates BinlogCoordinates
	GTIDPurged        string
}

type BinlogType int

const (
	BinaryLog BinlogType = iota
	RelayLog
)

type BackupPlugin interface {
	Backup() error
	Restore() error
	GetMetadata() (BackupMetadata, error)
}

type initializer func([]string, ...string) (BackupPlugin, error)

var ActiveSeeds = make(map[string]BackupPlugin)

var SeedMethods = map[string]initializer{
	//	"mydumper":          newMydumper,
	//	"mysqldump":         newMysqldump,
	"xtrabackup": newXtrabackup,
	//	"xtrabackup-stream": newXtrabackupStream,
}

func IntializePlugin(seedMethod string, databases []string, extra ...string) (BackupPlugin, error) {
	return SeedMethods[seedMethod](databases, extra...)
}
