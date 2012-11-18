package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var logPath = flag.String("d", "./logs", "Logs directory")

// log files have the following form: FU-<job-name>-<user>-<time>
type JobLogEntry struct {
	Path string // Path to the log entry
	Name string // job name
	User string // the user that started the
	Time int64  // when was it started
}

type JobLogEntries []*JobLogEntry

func (s JobLogEntries) Len() int      { return len(s) }
func (s JobLogEntries) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// ByTime implements sort.Interface by providing Less and using the Len and
// Swap methods of the embedded JobLogEntry value.
type ByTime struct{ JobLogEntries }

// most recent first
func (s ByTime) Less(i, j int) bool {
	return s.JobLogEntries[i].Time > s.JobLogEntries[j].Time
}

//Create a new log file, write the data and return the filepath
func NewLogEntry(job JobLogEntry, data []byte) (string, error) {
	newLogFile := "FU-" + job.Name + "-" + job.User + "-" + fmt.Sprintf("%d", job.Time)
	newLogFile = filepath.Join(*logPath, newLogFile)
	fd, err := os.Create(newLogFile)
	if err == nil {
		defer fd.Close()
		_, err = fd.Write(data)
		if err != nil {
			return newLogFile, err
		}
	} else {
		return "", err
	}
	return newLogFile, nil
}

// given the names of the log files return all the log job entries
// sorted by time
func LogEntries() JobLogEntries {
	var entries JobLogEntries
	filepath.Walk(*logPath,
		func(path string, info os.FileInfo, err error) error {
			// forward the error
			if err != nil {
				return err
			}
			// don't bother with logPath itself
			if path == *logPath {
				return nil
			}
			fileName := info.Name()
			if fileName[:2] == "FU" {
				parts := strings.Split(fileName, "-")
				jobName, userName, time := parts[1], parts[2], parts[3]
				timeInt, _ := strconv.ParseInt(time, 10, 64)
				logEntry := &JobLogEntry{
					Path: fileName,
					Name: jobName,
					User: userName,
					Time: timeInt,
				}
				entries = append(entries, logEntry)
			}
			return nil
		})
	// sort ByTime
	sort.Sort(ByTime{entries})
	return entries
}

/* Read the log entry file contents and return it */
func LogEntryBody(name string) ([]byte, error) {
	path := filepath.Join(*logPath, name)
	return ioutil.ReadFile(path)
}
