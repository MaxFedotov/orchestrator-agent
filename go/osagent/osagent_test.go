package osagent

import (
	"fmt"
	"io/ioutil"
	"math/rand"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	test "github.com/openark/golib/tests"
)

var (
	logFileExpected          = "mysql-bin.000007"
	positionExpected   int64 = 31669395
	gtidPurgedExpected       = "e88d3eec-4f8e-11e8-aa3b-6990953c6e71:1-124593"
	letters                  = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ ,;'-./()0123456789")
)

func getTestDataDir() (testDataDir string) {
	workingDir, _ := os.Getwd()
	for !strings.HasSuffix(workingDir, "orchestrator-agent") {
		workingDir = filepath.Dir(workingDir)
	}
	testDataDir = path.Join(workingDir, "tests/unit/")

	return testDataDir
}

func random(min int, max int) int {
	return rand.Intn(max-min) + min
}

func generateRandomString(length int) string {
	str := make([]rune, length)
	for i := range str {
		str[i] = letters[rand.Intn(len(letters))]
	}
	return string(str)
}

func generateTestDataForErrorLog(testDataDir string) error {
	var errorLog []byte
	rand.Seed(time.Now().UnixNano())
	errorLogRows := random(1, 100)
	var line = ""
	for i := 0; i <= errorLogRows; i++ {
		lineSymbols := random(10, 300)
		str := generateRandomString(lineSymbols)
		line = line + string(time.Now().Format("2006-01-02T15:04:05.553851Z")) + " 0 [Note] " + str + "\n"
	}
	errorLog = []byte(line)
	err := ioutil.WriteFile(path.Join(testDataDir, "mysqld.log"), errorLog, 0644)
	return err
}

func TestMySQLErrorLogTail(t *testing.T) {
	testDataDir := getTestDataDir()
	_ = generateTestDataForErrorLog(testDataDir)
	cmd := fmt.Sprintf(`tail -n 20 %s | cut -d "=" -f 2`, path.Join(testDataDir, "mysqld.log"))
	output, err := commandOutput(sudoCmd(cmd))
	linuxTail, err := outputLines(output, err)
	errorLogTail, _ := MySQLErrorLogTail(path.Join(testDataDir, "mysqld.log"))
	test.S(t).ExpectEquals(len(linuxTail), len(errorLogTail))
	for i := 0; i < len(linuxTail); i++ {
		test.S(t).ExpectEquals(strings.TrimSuffix(linuxTail[i], "\n"), strings.TrimSuffix(errorLogTail[i], "\n"))
	}
}

func TestParseMydumperMetadataGTID(t *testing.T) {
	testData := path.Join(getTestDataDir(), "metadata_parse/gtid")
	logFile, position, gtidPurged, _ := parseMydumperMetadata(testData)
	test.S(t).ExpectEquals(logFile, logFileExpected)
	test.S(t).ExpectEquals(position, positionExpected)
	test.S(t).ExpectEquals(gtidPurged, gtidPurgedExpected)
}
func TestParseMydumperMetadataPositional(t *testing.T) {
	testData := path.Join(getTestDataDir(), "metadata_parse/positional")
	logFile, position, gtidPurged, _ := parseMydumperMetadata(testData)
	test.S(t).ExpectEquals(logFile, logFileExpected)
	test.S(t).ExpectEquals(position, positionExpected)
	test.S(t).ExpectEquals(gtidPurged, "")
}

func TestParseXtrabackupMetadataGTID(t *testing.T) {
	testData := path.Join(getTestDataDir(), "metadata_parse/gtid")
	logFile, position, gtidPurged, _ := parseXtrabackupMetadata(testData)
	test.S(t).ExpectEquals(logFile, logFileExpected)
	test.S(t).ExpectEquals(position, positionExpected)
	test.S(t).ExpectEquals(gtidPurged, gtidPurgedExpected)
}
func TestParseXtrabackupMetadataPositional(t *testing.T) {
	testData := path.Join(getTestDataDir(), "metadata_parse/positional")
	logFile, position, gtidPurged, _ := parseXtrabackupMetadata(testData)
	test.S(t).ExpectEquals(logFile, logFileExpected)
	test.S(t).ExpectEquals(position, positionExpected)
	test.S(t).ExpectEquals(gtidPurged, "")
}

func TestParseMysqldumpMetadataGTID(t *testing.T) {
	testData := path.Join(getTestDataDir(), "metadata_parse/gtid")
	logFile, position, gtidPurged, _ := parseMysqldumpMetadata(testData)
	test.S(t).ExpectEquals(logFile, logFileExpected)
	test.S(t).ExpectEquals(position, positionExpected)
	test.S(t).ExpectEquals(gtidPurged, gtidPurgedExpected)
}
func TestParseMysqldumpMetadataPositional(t *testing.T) {
	testData := path.Join(getTestDataDir(), "metadata_parse/positional")
	logFile, position, gtidPurged, _ := parseMysqldumpMetadata(testData)
	test.S(t).ExpectEquals(logFile, logFileExpected)
	test.S(t).ExpectEquals(position, positionExpected)
	test.S(t).ExpectEquals(gtidPurged, "")
}
