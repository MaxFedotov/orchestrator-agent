package seed_test

import (
	"flag"
	"fmt"
	"os"
	"path"
	"testing"

	"github.com/github/orchestrator-agent/go/seed"
	"github.com/openark/golib/log"
	. "gopkg.in/check.v1"
)

func init() {
	log.SetLevel(log.DEBUG)
}

var testname = flag.String("testname", "TestGetMetadataPositional", "test name to run")

type MysqldumpTestSuite struct{}

var _ = Suite(&MysqldumpTestSuite{})

func Test(t *testing.T) { TestingT(t) }

func (s *MysqldumpTestSuite) SetUpTest(c *C) {
	if len(*testname) > 0 {
		if c.TestName() != fmt.Sprintf("MysqldumpTestSuite.%s", *testname) {
			c.Skip("skipping test due to not matched testname")
		}
	}
}

func (s *MysqldumpTestSuite) TestGetMetadataPositional(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	backupDir := path.Join(workingDir, "../../tests/functional/mysqldump")

	baseConfig := &seed.Base{
		BackupDir: backupDir,
	}
	mysqldump := &seed.MysqldumpSeed{
		Base:           baseConfig,
		BackupFileName: "orchestrator_agent_backup_positional.sql",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile: "mysql-bin.000005",
		LogPos:  68633362,
	}
	metadata, err := mysqldump.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)

}
