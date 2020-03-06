package cmd_test

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/orchestrator-agent/go/helper/cmd"
	. "gopkg.in/check.v1"
	"gopkg.in/pipe.v2"
)

func Test(t *testing.T) {
	TestingT(t)
}

type S struct{}

var _ = Suite(S{})

func (S) TestCommandRunSingleCmd(c *C) {
	err := cmd.CommandRun("ls -lah", false)
	c.Assert(err, IsNil)
}

func (S) TestCommandRunPipeCmd(c *C) {
	err := cmd.CommandRun("ls -lah | grep cmd", false)
	c.Assert(err, IsNil)
}

func (S) TestCommandOutputPipeCmd(c *C) {
	output, err := cmd.CommandOutput("echo HELLO WORLD | cut -f 2 -d \" \"", false)
	res := cmd.OutputLines(output)
	c.Assert(err, IsNil)
	c.Assert(res[0], Equals, "WORLD")
}

func (S) TestCommandOutputSingleCmd(c *C) {
	output, err := cmd.CommandOutput("echo HELLO WORLD", false)
	res := cmd.OutputLines(output)
	c.Assert(err, IsNil)
	c.Assert(res[0], Equals, "HELLO WORLD")
}

func (S) TestCommandRunPipeCmdWithRedirect(c *C) {
	path := filepath.Join(c.MkDir(), "cmd.txt")
	command := fmt.Sprintf("echo HELLO WORLD | cut -f 2 -d \" \" > %s", path)
	err := cmd.CommandRun(command, false)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadFile(path)
	c.Assert(string(data), Equals, "WORLD\n")
}

func (S) TestKill(c *C) {
	var activeCommands = make(map[string]*pipe.State)
	ch := make(chan error)
	go func() {
		ch <- cmd.CommandRunWithFunc("sleep 10s", false, func(cmd *pipe.State) {
			activeCommands["cmd1"] = cmd
		})
	}()
	time.Sleep(1 * time.Second)
	activeCommands["cmd1"].Kill()
	c.Assert(<-ch, ErrorMatches, "error: explicitly killed, stderr: ")
}
