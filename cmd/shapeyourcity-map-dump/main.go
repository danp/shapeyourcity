package main

import (
	"database/sql"
	"encoding/csv"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/peterbourgon/ff/v3"
)

type marker struct {
	id        string
	url       string
	createdAt time.Time
	address   string
	user      string
	category  string
	lat       string
	lng       string

	responses map[string]string
}

type responseField struct {
	name    string
	matcher *regexp.Regexp
}

func main() {
	fs := flag.NewFlagSet("shapeyourcity-dump", flag.ExitOnError)
	var (
		databaseFile = fs.String("database-file", "data.db", "data file path")
	)
	ff.Parse(fs, os.Args[1:])

	if *databaseFile == "" {
		fatalf("need -database-file")
	}

	if na := fs.NArg(); na > 0 && na%2 != 0 {
		fatalf("response field mappings must be in pairs, eg: your_comment '^Your Comment' what_should_happen '^What should happen'")
	}

	var responseFields []responseField
	args := fs.Args()
	for len(args) > 1 {
		name := args[0]
		matcher, err := regexp.Compile(args[1])
		if err != nil {
			fatalf("error parsing response field %q matcher %q: %w", name, args[1], err)
		}
		responseFields = append(responseFields, responseField{name: name, matcher: matcher})
		args = args[2:]
	}

	db, err := sql.Open("sqlite3", *databaseFile)
	if err != nil {
		fatalf("error opening data.db: %w", err)
	}
	defer db.Close()

	if err := dump(db, responseFields, os.Stdout); err != nil {
		fatalf("error dumping data: %w", err)
	}
}

func dump(db *sql.DB, responseFields []responseField, out io.Writer) error {
	rows, err := db.Query("select id, url, created_at, address, user, category, lat, lng from markers order by created_at")
	if err != nil {
		return err
	}

	markers := make(map[string]marker)
	for rows.Next() {
		var m marker
		if err := rows.Scan(&m.id, &m.url, &m.createdAt, &m.address, &m.user, &m.category, &m.lat, &m.lng); err != nil {
			return err
		}
		m.responses = make(map[string]string)
		markers[m.id] = m
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	rows, err = db.Query("select marker_id, question, answer from responses")
	if err != nil {
		return err
	}

	for rows.Next() {
		var id, q, a string
		if err := rows.Scan(&id, &q, &a); err != nil {
			return err
		}

		m := markers[id]

		for _, r := range responseFields {
			if r.matcher.MatchString(q) {
				m.responses[r.name] = html.UnescapeString(a)
				break
			}
		}

		markers[id] = m
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	markerIDs := make([]string, 0, len(markers))
	for id := range markers {
		markerIDs = append(markerIDs, id)
	}
	sort.Slice(markerIDs, func(i, j int) bool { return markers[markerIDs[i]].createdAt.Before(markers[markerIDs[j]].createdAt) })

	w := csv.NewWriter(out)

	responseFieldNames := make([]string, 0, len(responseFields))
	for _, r := range responseFields {
		responseFieldNames = append(responseFieldNames, r.name)
	}
	if err := w.Write(append(strings.Split("id url created_at address user category lat lng", " "), responseFieldNames...)); err != nil {
		return err
	}

	for _, id := range markerIDs {
		m := markers[id]

		resps := make([]string, 0, len(responseFields))
		for _, r := range responseFields {
			resps = append(resps, m.responses[r.name])
		}
		if err := w.Write(append([]string{m.id, m.url, m.createdAt.Format(time.RFC3339), m.address, m.user, m.category, m.lat, m.lng}, resps...)); err != nil {
			return err
		}
	}

	w.Flush()
	return w.Error()
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
