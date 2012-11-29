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

	dirInfoSlice, _ := dir.Readdir(-1)
	for _, fileinfo := range dirInfoSlice {
		entries = append(entries, fileinfo.Name())
	}
	return entries, nil
}

// validates the command to be run and returns it's execution path
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

/* Run the command and send back the results on channels */
func runCommand(command string, outChan chan string, errChan chan error) {
	// no matter what happens, close the channel
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

	outBuf := make([]byte, 1024)
	// read from the stdout to the buffer
	_, err = stdout.Read(outBuf)
	// while we have stuff to read from the output
	for err == nil {
		// send the output of  the command to the channel
		outChan <- string(outBuf)
		// read some more
		_, err = stdout.Read(outBuf)
	}
	// nothing more to send.. we can close the channel here
	close(outChan)

	// report any errors including the exit code of the command
	err = cmd.Wait()
	errChan <- err
}

/* /run/ - this handler will send the output of a running command */
func runHandler(response http.ResponseWriter, request *http.Request) {
	// forcing the output content type
	header := response.Header()

	// for some reason if text/plain is passed
	// Chrome thinks it's a application/octet-stream and tries
	// to download the log.
	header["Content-Type"] = []string{"text/html; charset=UTF-8"}
	header["Connection"] = []string{"close"}
	header["Vary"] = []string{"User-Agent"}

	jobName := request.URL.Path[len("/run/"):]
	// TODO: fix this and use the HTTP headers with user authentication
	userName := "Anonymous"
	startTime := time.Now().UTC()

	logEntry := JobLogEntry{
		Name: jobName,
		User: userName,
		Time: startTime.Unix(),
	}

	firstLine := "Started at " + startTime.Format("Mon Jan 2 15:04:05 -0700 MST 2006") + " by " + userName
	firstLine = firstLine + "\n==========================\n\n"
	// Adding a <pre> here because we want pretty output
	// the browser. We close it at end when we finished reading
	// from the command's output
	fmt.Fprintf(response, "<pre>"+firstLine)
	logFilePath, err := NewLogEntry(logEntry, firstLine)
	if err != nil {
		log.Print("Failed to create a new log entry: ", err)
	}

	log.Print("Started job: " + jobName)
	// We'll use two channels. The error and the command output channel
	// the error channel is used to get the errors thrown while running
	// the job and the output channel is for returning the output of 
	// the command
	outChan := make(chan string)
	errChan := make(chan error)
	go runCommand(jobName, outChan, errChan)

	// TODO: perhaps implement some kind of timeout?
	for {
		select {
		case content, closed := <-outChan:
			AppendLog(logFilePath, content)
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
			errStr := err.Error()
			if err != nil {
				// TODO: maybe there is another way to get the 
				// exit status?
				if strings.Contains(errStr, "exit status") {
					// Ex: exit status 0
					msgparts := strings.Split(err.Error(), " ")
					// Ex: we get "0" here
					status := msgparts[2]
					// Rename the log file to store 
					// the exit status
					logFilePath = RenameLogFile(logFilePath, time.Since(startTime).Seconds(), status)
				}
			}

			AppendLog(logFilePath, errStr)

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
