package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//execute commands only from this directory
var cmdDir = flag.String("c", "./cmds", "Commands dir")

// return a list of avaiable jobs that can be run
func jobEntries() ([]string, error) {
	var entries []string
	dir, err := os.Open(*cmdDir)
	if err != nil {
		return entries, err
	}
	defer dir.Close()

	// this reads the whole content of the dir. It may not be a good idea.
	dirInfoSlice, _ := dir.Readdir(-1)
	for _, fileinfo := range dirInfoSlice {
		entries = append(entries, fileinfo.Name())
	}
	return entries, nil
}

// validates the commands and returns it's execution path
func validateCmd(cmd string) (string, error) {
	jobs, err := jobEntries()
	if err != nil {
		return "", err
	}
	for _, job := range jobs {
		if cmd == job {
			return filepath.Join(*cmdDir, cmd), nil
		}
	}
	return "", errors.New("Command not found")
}

/* Run the command and send back the results on a channel */
func runCommand(command string, outChan chan string, errChan chan error) {
	// first check that we have the command file in the right place

	// no matter what happens, close the channels
	defer close(errChan)

	cmdPath, err := validateCmd(command)
	if err != nil {
		errChan <- err
		log.Print("Invalid command: ", err)
		return
	}

	cmd := exec.Command(cmdPath)
	// TODO: handle Stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		errChan <- err
		return
	}

	// run the command, but don't block
	if err := cmd.Start(); err != nil {
		errChan <- err
		return
	}

	// Read from .
	outBuf := make([]byte, 1024)
	_, err = stdout.Read(outBuf)
	// while we have stuff to read from the output 
	for err == nil {
		// send the output of  the command to the channel
		outChan <- string(outBuf)
		// reading some more
		_, err = stdout.Read(outBuf)
	}
	// nothing more to send.. we can close the channel here
	close(outChan)

	// report any errors
	err = cmd.Wait()
	errChan <- err
}

/* /run/ - this handler will send the output of a running command */
func runHandler(response http.ResponseWriter, r *http.Request) {
	// forcing the output content type
	header := response.Header()

	// for some reason if text/plain is passed
	// Chrome thinks it's a application/octet-stream and tries
	// to download the log.
	header["Content-Type"] = []string{"text/html; charset=UTF-8"}
	header["Connection"] = []string{"close"}
	header["Vary"] = []string{"User-Agent"}

	jobName := r.URL.Path[len("/run/"):]

	userName := "Anonymous"
	startTime := time.Now().UTC() // Always use UTC time
	logEntry := JobLogEntry{
		Name: jobName,
		User: userName,
		Time: startTime.Unix(),
	}

	firstLine := "Started at " + startTime.Format("Mon Jan 2 15:04:05 -0700 MST 2006") + " by " + userName
	firstLine = firstLine + "\n==========================\n\n"
	fmt.Fprintf(response, "<pre>"+firstLine)
	logFilePath, err := NewLogEntry(logEntry, firstLine)
	if err != nil {
		log.Print("Failed to create a new log entry: ", err)
	}

	// we'll launch the command in a goroutine
	outChan := make(chan string)
	errChan := make(chan error)
	go runCommand(jobName, outChan, errChan)
	// we have two channels. The error and the command output channel
	// the error channel is used to get the errors thrown while running
	// the job and the output channel is for returning the output
	for {
		select {
		case content, closed := <-outChan:
			// os.APPEND does not work here for some reason
			log.Print("Writing content to " + logFilePath)
			logFileFd, _ := os.OpenFile(logFilePath, os.O_WRONLY, 0666)
			// go to the end of the file
			logFileFd.Seek(0, os.SEEK_END)
			logFileFd.WriteString(content)
			logFileFd.Close()

			fmt.Fprintf(response, content)
			response.(http.Flusher).Flush()

			if !closed {
				secondsSince := time.Since(startTime).Seconds()
				logFilePath = RenameLogFile(logFilePath, secondsSince, "0")
				fmt.Fprintf(response, "</pre>") // close <pre>
				log.Print("Finished job: " + jobName)
			}
		case err, _ := <-errChan:
			log.Print("Received an error: ", err)
			// os.APPEND does not work here for some reason
			// go to the end of the file
			errStr := err.Error()
			if err != nil {
				if strings.Contains(errStr, "exit status") {
					msgparts := strings.Split(err.Error(), " ")
					status := msgparts[2]
					logFilePath = RenameLogFile(logFilePath, time.Since(startTime).Seconds(), status)
				}
			}

			log.Print("Opening the log file: " + logFilePath)
			logFileFd, _ := os.OpenFile(logFilePath, os.O_WRONLY, 0666)
			logFileFd.Seek(0, os.SEEK_END)
			logFileFd.WriteString(errStr)
			logFileFd.Close()

			fmt.Fprintf(response, errStr)
			response.(http.Flusher).Flush()
			return
		}
	}
}

// update the log filename with the new duration and 
func RenameLogFile(filepath string, duration float64, status string) string {
	parts := strings.Split(filepath, "-")
	durationStr := strconv.FormatFloat(duration, 'f', 0, 64)

	parts[4] = durationStr
	parts[5] = status
	newLogPath := strings.Join(parts, "-")
	err := os.Rename(filepath, newLogPath)
	if err != nil {
		log.Print("Failed to rename log file ", err)
	}
	return newLogPath
}

/* /logs will return the latest logs ordered by date from the logs folder */
func logsHandler(w http.ResponseWriter, r *http.Request) {
	// forcing the output content type
	header := w.Header()
	header["Content-Type"] = []string{"application/json"}
	// if we have a name of the log then we should get the contents of the log
	var dataJson []byte
	var err error
	if r.FormValue("name") != "" {
		data := make(map[string]string, 1)
		body, err := LogEntryBody(r.FormValue("name"))
		if err != nil {
			log.Print("Failed to get log entry body ", err)
			fmt.Fprintf(w, err.Error())
		}

		data["body"] = string(body)
		dataJson, err = json.Marshal(data)
		if err != nil {
			log.Print("Failed to encode json: ", err)
		}
	} else {
		job := r.FormValue("job")
		data := LogEntries(job)
		dataJson, err = json.Marshal(data)
		if err != nil {
			log.Print("Failed to encode json: ", err)
		}
	}
	fmt.Fprintf(w, string(dataJson))
}

/* /listJobs will display the available jobs that can be run */
func jobsHandler(w http.ResponseWriter, r *http.Request) {
	// forcing the output content type
	header := w.Header()
	header["Content-Type"] = []string{"application/json"}

	entries, err := jobEntries()
	if err != nil {
		log.Print("Error loading available jobs: ", err)
	}
	dataJson, err := json.Marshal(entries)
	if err != nil {
		log.Print("Failed to encode json")
	}
	fmt.Fprintf(w, string(dataJson))
}

/* This is a helper wrapper. Allows us to log some stuff */
func DefaultWrapper(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", r.RemoteAddr, r.Method, r.URL)
		handler.ServeHTTP(w, r)
	})
}

func main() {
	http.HandleFunc("/run/", runHandler)

	http.HandleFunc("/logs", logsHandler)
	http.HandleFunc("/jobs", jobsHandler)

	// serve other static stuff
	http.Handle("/", http.StripPrefix("/",
		http.FileServer(http.Dir("./static"))))

	// command running handler
	// server index.html at the end
	port := ":8000"
	log.Printf("Starting on " + port)
	err := http.ListenAndServe(port, DefaultWrapper(http.DefaultServeMux))
	if err != nil {
		log.Fatal(err)
	}
}
