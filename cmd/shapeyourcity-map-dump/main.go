package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"html"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/danp/shapeyourcity/internal/store"
	_ "github.com/mattn/go-sqlite3"
	"github.com/peterbourgon/ff/v3"
)

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

	st, err := store.NewDB(*databaseFile)
	if err != nil {
		fatalf("error initializing store: %w", err)
	}

	err = dump(context.Background(), st, responseFields, os.Stdout)

	st.Close()

	if err != nil {
		fatalf("error dumping data: %w", err)
	}
}

type responseField struct {
	name    string
	matcher *regexp.Regexp
}

func (f responseField) match(q string) bool {
	return f.matcher.MatchString(q)
}

func dump(ctx context.Context, st *store.DB, responseFields []responseField, out io.Writer) error {
	markers, err := st.Markers(ctx)
	if err != nil {
		return err
	}
	sort.Slice(markers, func(i, j int) bool { return markers[i].CreatedAt.Before(markers[j].CreatedAt) })

	markerResponseFields := make(map[int]map[string]string)
	for _, m := range markers {
		for _, mr := range m.Responses {
			for _, rf := range responseFields {
				if rf.match(mr.Question) {
					if markerResponseFields[m.ID] == nil {
						markerResponseFields[m.ID] = make(map[string]string)
					}
					markerResponseFields[m.ID][rf.name] = html.UnescapeString(string(mr.Answer))
					break
				}
			}
		}
	}

	w := csv.NewWriter(out)

	header := strings.Split("id url created_at address user category lat lng", " ")
	for _, r := range responseFields {
		header = append(header, r.name)
	}
	if err := w.Write(header); err != nil {
		return err
	}

	for _, m := range markers {
		resps := make([]string, 0, len(responseFields))
		for _, r := range responseFields {
			resps = append(resps, markerResponseFields[m.ID][r.name])
		}

		fields := []string{
			strconv.Itoa(m.ID),
			m.URL,
			m.CreatedAt.Format(time.RFC3339),
			m.Address,
			m.User,
			m.Category,
			m.Latitude,
			m.Longitude,
		}
		fields = append(fields, resps...)

		if err := w.Write(fields); err != nil {
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
