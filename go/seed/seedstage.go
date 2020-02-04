package seed

import (
	"bytes"
	"time"

	"gopkg.in/pipe.v2"
)

type Stage int

const (
	Prepare Stage = iota
	Backup
	Restore
	Cleanup
)

func (s Stage) String() string {
	return [...]string{"Prepare", "Backup", "Restore", "Cleanup"}[s]
}

func (s Stage) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString(`"`)
	buffer.WriteString(s.String())
	buffer.WriteString(`"`)
	return buffer.Bytes(), nil
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

type StageStatus struct {
	Stage      Stage
	StartedAt  time.Time
	Status     Status
	Details    string
	Command    *pipe.State       `json:"-"`
	StatusChan chan *StageStatus `json:"-"`
}

func NewSeedStage(stage Stage, statusChan chan *StageStatus) *StageStatus {
	seedStage := &StageStatus{
		Stage:      stage,
		StartedAt:  time.Now(),
		Status:     Running,
		StatusChan: statusChan,
	}
	seedStage.StatusChan <- seedStage
	return seedStage
}

func (s *StageStatus) UpdateSeedStatus(status Status, command *pipe.State, details string) {
	s.Status = status
	s.Command = command
	s.Details = details
	s.StatusChan <- s
}
