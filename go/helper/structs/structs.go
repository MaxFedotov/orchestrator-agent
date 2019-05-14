package structs

type BackupMetadata struct {
	LogFile        string
	LogPos         int64
	GtidExecuted   string
	MasterUser     string // This is optional field. If it is empty, will be read from configuration file
	MasterPassword string // This is optional field. If it is empty, will be read from configuration file
}

type AgentParams struct {
	MysqlUser     string
	MysqlPassword string
	MysqlPort     int
	MysqlDatadir  string
	BackupFolder  string
	InnoDBLogDir  string
}
