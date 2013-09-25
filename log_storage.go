package main

import (
	"database/sql"
	"flag"
	_ "github.com/mattn/go-sqlite3"
	"log"
	"time"
)

var DbPath = flag.String("db", "/tmp/db.sqlite3", "Logs database")

type JobLogEntry struct {
	Id     string // job id
	Name   string // job name
	User   string // the user that started the
	Start  time.Time
	End    time.Time
	Body   string // when was it started
	Status int64  // 0-255 - status code returned by the job
}

type JobLogEntries []*JobLogEntry

// Create a new database and it's schema if it doesn't exist and return the db
func getDB() *sql.DB {
	db, err := sql.Open("sqlite3", *DbPath)
	if err != nil {
		log.Fatal(err)
	}

	sql := `create table if not exists log (
        id integer not null primary key autoincrement,
        name text,
        user text,
        start_dt datetime,
        end_dt datetime default null,
        body text default "",
        status integer default null
    )`
	_, err = db.Exec(sql)
	if err != nil {
		log.Fatal("%q: %s\n", err, sql)
	}
	return db
}

//Create a new log and return it's id
func NewLogEntry(job JobLogEntry) string {
	db := getDB()
	defer db.Close()

	sql := "insert into log (name, user, start_dt) values (?, ?, ?)"
	_, err := db.Exec(sql, job.Name, job.User, job.Start)

	if err != nil {
		log.Fatalf("%q: %s\n", err, sql)
	}

	id := ""
	err = db.QueryRow("select last_insert_rowid()").Scan(&id)
	if err != nil {
		log.Fatal(err)
	}
	return id
}

// Given the names of the job return all the log job entries ordered by time
func LogEntries(job string) JobLogEntries {
	var entries JobLogEntries

	db := getDB()
    defer db.Close()

	// datetime is not parsed correctly for some reason.
	rows, err := db.Query(`select id, name, user, start_dt, end_dt, status
        from log`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	for rows.Next() {
		jobLog := new(JobLogEntry)
        start := new(time.Time)
        end := new(time.Time)
		rows.Scan(&jobLog.Id, &jobLog.Name, &jobLog.User, &start, &end,
            &jobLog.Status)
        jobLog.Start = *start
        if end != nil {
            jobLog.End = *end
        }
		entries = append(entries, jobLog)
	}
	return entries
}

// Read the log entry file contents and return it
func LogEntryBody(id string) (string, error) {
	body := ""
	db := getDB()
	defer db.Close()

	err := db.QueryRow("select body from log where id = ?", id).Scan(&body)
	return body, err
}

// append a string to a log file
func AppendLog(id, body string) {
	db := getDB()
	defer db.Close()

    oldBody := ""
    if (body != ""){
        err := db.QueryRow("select body from log where id = ?", id).Scan(&oldBody)
        if err != nil {
            return;
        }
        body = oldBody + body

        sql := "update log set body = ? where id = ?"
        db.Exec(sql, body, id)
    }
}

// update the log filename with the new duration and
func UpdateLog(id string, endTime time.Time, status string) {
	db := getDB()
	defer db.Close()
	sql := "update log set end = ?, status = ? where id = ?"
	db.Exec(sql, endTime.String(), status, id)
}

