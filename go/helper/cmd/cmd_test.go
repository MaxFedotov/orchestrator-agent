package cmd_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	log "github.com/sirupsen/logrus"
	. "gopkg.in/check.v1"
	"gopkg.in/pipe.v2"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct {
	CmdOpts *cmd.CmdOpts
}

var _ = Suite(&S{})

func (s *S) SetUpTest(c *C) {
	var cmdOpts = cmd.NewCmd(false, "", log.WithFields(log.Fields{"prefix": "CMD"}))
	s.CmdOpts = cmdOpts
}

func (s *S) TestCommandRunSingleCmd(c *C) {
	err := s.CmdOpts.CommandRun("ls -lah")
	c.Assert(err, IsNil)
}

func (s *S) TestCommandRunPipeCmd(c *C) {
	err := s.CmdOpts.CommandRun("ls -lah | grep cmd")
	c.Assert(err, IsNil)
}

func (s *S) TestCommandOutputPipeCmd(c *C) {
	output, err := s.CmdOpts.CommandOutput("echo HELLO WORLD | cut -f 2 -d \" \"")
	res := s.CmdOpts.OutputLines(output)
	c.Assert(err, IsNil)
	c.Assert(res[0], Equals, "WORLD")
}

func (s *S) TestCommandOutputSingleCmd(c *C) {
	output, err := s.CmdOpts.CommandOutput("echo HELLO WORLD")
	res := s.CmdOpts.OutputLines(output)
	c.Assert(err, IsNil)
	c.Assert(res[0], Equals, "HELLO WORLD")
}

func (s *S) TestCommandRunPipeCmdWithRedirect(c *C) {
	path := filepath.Join(c.MkDir(), "cmd.txt")
	command := fmt.Sprintf("echo HELLO WORLD | cut -f 2 -d \" \" > %s", path)
	err := s.CmdOpts.CommandRun(command)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadFile(path)
	c.Assert(string(data), Equals, "WORLD\n")
}

func (s *S) TestKill(c *C) {
	var activeCommands = make(map[string]*pipe.State)
	ch := make(chan error)
	go func() {
		ch <- s.CmdOpts.CommandRunWithFunc("sleep 10s", func(cmd *pipe.State) {
			activeCommands["cmd1"] = cmd
		})
	}()
	time.Sleep(1 * time.Second)
	activeCommands["cmd1"].Kill()
	c.Assert(<-ch, ErrorMatches, "error: explicitly killed, stderr: ")
}
