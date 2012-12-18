package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// execute commands only from this directory
var cmdDir = flag.String("c", "./cmds", "Commands dir")

// this lock is used to not allow 2 commands to run at once
var commandLock *sync.Mutex

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

// this is a combined output channel used for both stdout and stderr pipes
type combinedOutput struct {
	data []byte
	exit bool // use this to signal that we ended reading from the pipe
}

// read from pipe (stdout/stderr) and send back using channel
func readPipe(pipe io.ReadCloser, pipeChan chan combinedOutput) {
	buf := make([]byte, 1024)
	_, err := pipe.Read(buf)

	var out combinedOutput
	for err == nil {
		// send the output of  the command to the channel
		out.data = buf
		out.exit = false
		pipeChan <- out

		// read some more
		_, err = pipe.Read(buf)
	}
	out.data = nil
	out.exit = true
	pipeChan <- out
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
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		errChan <- err
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		errChan <- err
		return
	}

	// run the command, but don't block
	if err := cmd.Start(); err != nil {
		errChan <- err
		return
	}

	// combine the out from both pipes
	comChan := make(chan combinedOutput)
	go readPipe(stdout, comChan)
	go readPipe(stderr, comChan)

	count := 0
	for out := range comChan {
		if out.exit == true {
			if count == 1 {
				//close(comChan)
				break
			}
			count++
		}
		outChan <- string(out.data)
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

	// aquiring the lock. Should block here
	commandLock.Lock()
	defer commandLock.Unlock()
 
	jobName := request.URL.Path[len("/run/"):]
	// TODO: fix this and use the HTTP headers with user authentication
	userName := "Anonymous"
	
	// start counting
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
	response.(http.Flusher).Flush()


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

			if !closed {
				secondsSince := time.Since(startTime).Seconds()
				logFilePath = RenameLogFile(logFilePath, secondsSince, "0")
				fmt.Fprintf(response, "</pre>") // close <pre>
				log.Print("Finished job: " + jobName)
				return
			}
			response.(http.Flusher).Flush()
		case err, _ := <-errChan:
			errStr := ""
			if err != nil {
				errStr = err.Error()
				// TODO: maybe there is another way to get the 
				// exit status?
				if strings.Contains(errStr, "exit status") {
					// Ex: exit status 0
					msgparts := strings.Split(err.Error(), " ")
					// Ex: we get "0" here
					status := msgparts[2]
					// Rename the log file to store 
					// the exit status
					secondsSince := time.Since(startTime).Seconds()
					logFilePath = RenameLogFile(logFilePath,
						secondsSince, status)

					log.Print("Finished job: " + jobName)
					return
				}
			}

			AppendLog(logFilePath, errStr)

			fmt.Fprintf(response, errStr)
			response.(http.Flusher).Flush()
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

// This will be useful for some pagination
type ResponseLogs struct {
	Entries JobLogEntries
	Length int
}

/* /logs will return the latest logs ordered by date from the logs folder */
func logsHandler(w http.ResponseWriter, r *http.Request) {
	// forcing the output content type
	header := w.Header()
	header["Content-Type"] = []string{"application/json"}
	// if we have a name of the log then we should get the contents of the log
	var dataJson []byte
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
		start := 0
		offset := 50 // number of items per page
		job := r.FormValue("job") // filter by job is any
		logEntries := LogEntries(job)
		logEntriesLen := len(logEntries)
		if offset >= logEntriesLen {
			offset = logEntriesLen
		}

		resp := ResponseLogs{
			Entries: logEntries[start:offset],
			Length: logEntriesLen,
		}

		page, err := strconv.ParseInt(r.FormValue("page"), 10, 32);
		if  err == nil {
			start = int(page) * offset
			if start >= resp.Length {
				start = resp.Length - offset
			}
			offset += start
			if offset >= resp.Length {
				offset = resp.Length
			}
			resp.Entries = resp.Entries[start:offset]
		}

		dataJson, err = json.Marshal(resp)
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
	// create the lock
	commandLock = new(sync.Mutex)
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
