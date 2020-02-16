package seed

import (
	"bytes"
	"os"
	"time"

	"gopkg.in/pipe.v2"
)

type Stage int

const (
	Prepare Stage = iota
	Backup
	Restore
	Cleanup
	ConnectSlave
)

func (s Stage) String() string {
	return [...]string{"Prepare", "Backup", "Restore", "Cleanup", "ConnectSlave"}[s]
}

func (s Stage) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(s.String())
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

var ToSeedStage = map[string]Stage{
	"Prepare":      Prepare,
	"Backup":       Backup,
	"Restore":      Restore,
	"Cleanup":      Cleanup,
	"ConnectSlave": ConnectSlave,
}

type Status int

const (
	Running Status = iota
	Completed
	Error
)

func (s Status) String() string {
	return [...]string{"Running", "Completed", "Error"}[s]
}

func (s Status) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(s.String())
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
}

type SeedStageState struct {
	Stage      Stage
	Hostname   string
	Timestamp  time.Time
	Status     Status
	Details    string
	Command    *pipe.State          `json:"-"`
	StatusChan chan *SeedStageState `json:"-"`
}

func NewSeedStage(stage Stage, statusChan chan *SeedStageState) *SeedStageState {
	seedStageState := &SeedStageState{
		Stage:      stage,
		Timestamp:  time.Now(),
		Status:     Running,
		StatusChan: statusChan,
	}
	seedStageState.Hostname, _ = os.Hostname()
	seedStageState.StatusChan <- seedStageState
	return seedStageState
}

func (s *SeedStageState) UpdateSeedStatus(status Status, command *pipe.State, details string) {
	s.Status = status
	s.Command = command
	s.Details = details
	s.Timestamp = time.Now()
	s.StatusChan <- s
}
