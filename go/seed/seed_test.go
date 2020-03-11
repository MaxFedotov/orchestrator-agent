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

var testname = flag.String("testname", "", "test name to run")

type SeedTestSuite struct{}

var _ = Suite(&SeedTestSuite{})

func Test(t *testing.T) { TestingT(t) }

func (s *SeedTestSuite) SetUpTest(c *C) {
	if len(*testname) > 0 {
		if c.TestName() != fmt.Sprintf("SeedTestSuite.%s", *testname) {
			c.Skip("skipping test due to not matched testname")
		}
	}
}

func (s *SeedTestSuite) TestMysqldumpGetMetadataPositional(c *C) {
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

func (s *SeedTestSuite) TestMysqldumpGetMetadataGtid(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	backupDir := path.Join(workingDir, "../../tests/functional/mysqldump")

	baseConfig := &seed.Base{
		BackupDir: backupDir,
	}
	mysqldump := &seed.MysqldumpSeed{
		Base:           baseConfig,
		BackupFileName: "orchestrator_agent_backup_gtid.sql",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile:      "mysql-bin.000005",
		LogPos:       68633362,
		GtidExecuted: "70f9ba7b-5ee3-11ea-96a0-5254008afee6:1",
	}
	metadata, err := mysqldump.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)
}

func (s *SeedTestSuite) TestMydumperGetMetadataPositional(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	backupDir := path.Join(workingDir, "../../tests/functional/mydumper")

	baseConfig := &seed.Base{
		BackupDir: backupDir,
	}
	mydumper := &seed.MydumperSeed{
		Base:             baseConfig,
		MetadataFileName: "metadata_positional",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile: "mysql-bin.000022",
		LogPos:  194,
	}
	metadata, err := mydumper.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)
}

func (s *SeedTestSuite) TestMydumperGetMetadataGtid(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	backupDir := path.Join(workingDir, "../../tests/functional/mydumper")

	baseConfig := &seed.Base{
		BackupDir: backupDir,
	}
	mydumper := &seed.MydumperSeed{
		Base:             baseConfig,
		MetadataFileName: "metadata_gtid",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile:      "mysql-bin.000022",
		LogPos:       194,
		GtidExecuted: "5c2bd8fc-5ee3-11ea-adf4-5254008afee6:1-741",
	}
	metadata, err := mydumper.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)
}

func (s *SeedTestSuite) TestXtrabackupGetMetadataPositional(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	datadir := path.Join(workingDir, "../../tests/functional/xtrabackup")

	baseConfig := &seed.Base{
		MySQLDatadir: datadir,
	}
	xtrabackup := &seed.XtrabackupSeed{
		Base:             baseConfig,
		MetadataFileName: "xtrabackup_binlog_info",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile: "mysql-bin.000007",
		LogPos:  2030155,
	}
	metadata, err := xtrabackup.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)
}

func (s *SeedTestSuite) TestXtrabackupGetMetadataGtid(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	datadir := path.Join(workingDir, "../../tests/functional/xtrabackup")

	baseConfig := &seed.Base{
		MySQLDatadir: datadir,
	}
	xtrabackup := &seed.XtrabackupSeed{
		Base:             baseConfig,
		MetadataFileName: "xtrabackup_binlog_info_gtid",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile:      "mysql-bin.000009",
		LogPos:       325,
		GtidExecuted: "956ddec0-33c0-11ea-8a71-3738749ebac9:1",
	}
	metadata, err := xtrabackup.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)
}

func (s *SeedTestSuite) TestLVMGetMetadataPositional(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	datadir := path.Join(workingDir, "../../tests/functional/lvm")

	baseConfig := &seed.Base{
		MySQLDatadir: datadir,
	}
	lvm := &seed.LVMSeed{
		Base:             baseConfig,
		MetadataFileName: "metadata_positional",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile: "mysql-bin.000009",
		LogPos:  701,
	}
	metadata, err := lvm.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)
}

func (s *SeedTestSuite) TestLVMGetMetadataGtidl(c *C) {
	workingDir, err := os.Getwd()
	c.Assert(err, IsNil)
	datadir := path.Join(workingDir, "../../tests/functional/lvm")

	baseConfig := &seed.Base{
		MySQLDatadir: datadir,
	}
	lvm := &seed.LVMSeed{
		Base:             baseConfig,
		MetadataFileName: "metadata_gtid",
	}
	seedMetadata := &seed.SeedMetadata{
		LogFile:      "mysql-bin.000009",
		LogPos:       701,
		GtidExecuted: "5c2bd8fc-5ee3-11ea-adf4-5254008afee6:1-741",
	}
	metadata, err := lvm.GetMetadata()
	c.Assert(err, IsNil)
	c.Assert(metadata, DeepEquals, seedMetadata)
}
