/* */
package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
		log.Print(err)
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
	if err := cmd.Wait(); err != nil {
		errChan <- err
		return
	}
}

/* /run/ - this handler will send the output of a running command */
func runHandler(response http.ResponseWriter, r *http.Request) {
	// forcing the output content type
	header := response.Header()
	header["Content-Type"] = []string{"text/plain"}

	// we'll launch the command in a goroutine
	outChan := make(chan string)
	errChan := make(chan error)
	jobName := r.URL.Path[len("/run/"):]
	go runCommand(jobName, outChan, errChan)

	userName := "Anonymous"
	startTime := time.Now().UTC() // Always use UTC time
	logEntry := JobLogEntry{
		Name: jobName,
		User: userName,
		Time: startTime.Unix(),
	}

	firstLine := "Started at " + startTime.Format("Mon Jan 2 15:04:05 -0700 MST 2006") + " by " + userName
	sep := "\n==========================\n\n"
	fmt.Fprintf(response, firstLine)
	logFilePath, err := NewLogEntry(logEntry, []byte(firstLine+sep))
	if err != nil {
		log.Fatal("Failed to create a new log entry: ", err)
	}
	logFileFd, err := os.OpenFile(logFilePath, os.O_APPEND, 0600)
	defer logFileFd.Close()

	// we have two channels. The error and the command output channel
	// the error channel is used to get the errors thrown while running
	// the job and the output channel is for returning the output
	for {
		select {
		case content, closed := <-outChan:
			if !closed {
				log.Print("Finished job: " + jobName)
				return
			}
			logFileFd.Write([]byte(content))
			fmt.Fprintf(response, content)
			response.(http.Flusher).Flush()
		case err, _ := <-errChan:
			log.Print(err)
			errStr := "INTERNAL: " + err.Error()
			logFileFd.Write([]byte(errStr))

			fmt.Fprintf(response, errStr)
			response.(http.Flusher).Flush()
			return
		}
	}
}

func toBase64(data string) string {
	var buf bytes.Buffer
	encoder := base64.NewEncoder(base64.StdEncoding, &buf)
	encoder.Write([]byte(data))
	encoder.Close()
	return buf.String()
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
		data["body"] = string(body)
		if err != nil {
			log.Print(err)
			fmt.Fprintf(w, err.Error())
		}
		dataJson, err = json.Marshal(data)
		if err != nil {
			log.Print("Failed to encode json: ", err)
		}
	} else {
		data := LogEntries()
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
		log.Printf("Error loading available jobs: ", err)
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
