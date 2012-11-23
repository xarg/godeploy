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

// log files have the following form: GD-<job-name>-<user>-<time>-<duration>
type JobLogEntry struct {
	Path string // Path to the log entry
	Name string // job name
	User string // the user that started the
	Time int64  // when was it started
	Duration float64// duration in seconds
	Status int64  // 0-255 - status code returned by the job
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
func NewLogEntry(job JobLogEntry, data string) (string, error) {
	newLogFile := "GD-" + job.Name 
	newLogFile += "-" + job.User 
	newLogFile += "-" + fmt.Sprintf("%d", job.Time)
	newLogFile += "-" + fmt.Sprintf("%d", int(job.Duration))
	newLogFile += "-" + fmt.Sprintf("%d", job.Status)

	newLogFile = filepath.Join(*logPath, newLogFile)
	fd, err := os.Create(newLogFile)
	defer fd.Close()
	if err == nil {
		_, err = fd.WriteString(data)
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
func LogEntries(job string) JobLogEntries {
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
			if fileName[:2] == "GD" {
				parts := strings.Split(fileName, "-")
				jobName, userName, time, duration, status := parts[1], parts[2], parts[3], parts[4], parts[5]
				if job != "" && jobName != job {
					// filter by job's name if set
					return nil
				}
				timeInt, _ := strconv.ParseInt(time, 10, 64)
				durationFloat, _ := strconv.ParseFloat(duration, 64)
				statusInt, _ := strconv.ParseInt(status, 10, 64)
				logEntry := &JobLogEntry{
					Path: fileName,
					Name: jobName,
					User: userName,
					Time: timeInt,
					Duration: durationFloat,
					Status: statusInt,
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
