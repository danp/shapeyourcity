package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/peterbourgon/ff/v3"
)

func main() {
	fs := flag.NewFlagSet("shapeyourcity-sync", flag.ExitOnError)
	var (
		databaseFile = fs.String("database-file", "data.db", "data file path")
		baseURLA     = fs.String("base-url", "", "base URL, eg https://www.shapeyourcityhalifax.ca/peninsula-south-complete-streets/maps/peninsula-south-complete-streets")
	)
	ff.Parse(fs, os.Args[1:])

	if *databaseFile == "" {
		fatalf("need -database-file")
	}

	if *baseURLA == "" {
		fatalf("need -base-url")
	}

	db, err := sql.Open("sqlite3", *databaseFile)
	if err != nil {
		fatalf("error opening data.db: %s", err)
	}
	defer db.Close()

	if err := sync(db, *baseURLA); err != nil {
		fatalf("error syncing: %s", err)
	}
}

func sync(db *sql.DB, baseURLA string) error {
	_, err := db.Exec("create table if not exists markers (id text not null primary key, url text, address text, category text, created_at datetime, editable bool, lat text, lng text, user text)")
	if err != nil {
		return fmt.Errorf("creating markers table: %w", err)
	}
	_, err = db.Exec("create table if not exists responses (marker_id integer not null, mode text not null, question_type text not null, question text not null, answer text, foreign key(marker_id) references markers(id))")
	if err != nil {
		return fmt.Errorf("creating responses table: %w", err)
	}

	baseURL, err := url.Parse(baseURLA)
	if err != nil {
		return fmt.Errorf("parsing base URL: %w", err)
	}
	if p := baseURL.Path; p[len(p)-1] != '/' {
		baseURL.Path += "/"
	}

	markersURL, err := baseURL.Parse("markers?filter=other_users")
	if err != nil {
		return fmt.Errorf("parsing markers URL: %w", err)
	}

	b, err := get(markersURL.String())
	if err != nil {
		return fmt.Errorf("getting markers: %w", err)
	}

	var data struct {
		Markers []marker
	}

	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("unmarshaling markers: %w", err)
	}

	knownIDs := make(map[int]bool)
	rows, err := db.Query("select id, editable from markers")
	if err != nil {
		return fmt.Errorf("finding current marker ids: %w", err)
	}
	for rows.Next() {
		var id int
		var editable bool
		if err := rows.Scan(&id, &editable); err != nil {
			return fmt.Errorf("finding current marker ids: %w", err)
		}
		knownIDs[id] = editable
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("finding current marker ids: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("finding current marker ids: %w", err)
	}

	markers := data.Markers
	for _, m := range markers {
		editable, known := knownIDs[m.ID]
		if known && !editable {
			continue
		}

		murl, err := baseURL.Parse("#marker-" + strconv.Itoa(m.ID))
		if err != nil {
			return fmt.Errorf("building direct marker URL: %w", err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("adding marker: %w", err)
		}

		if !known {
			_, err := tx.Exec(
				"insert into markers (id, url, address, category, created_at, editable, lat, lng, user) values (?, ?, ?, ?, ?, ?, ?, ?, ?)",
				m.ID,
				murl.String(),
				m.Address,
				m.Category.Name,
				m.CreatedAt.UTC(),
				m.Editable,
				m.Lat,
				m.Lng,
				m.User.Login,
			)
			if err != nil {
				return fmt.Errorf("adding marker: %w", err)
			}
			log.Println("discovered marker ID", m.ID)
		} else if !m.Editable {
			_, err := tx.Exec("update markers set editable=0 where id=?", m.ID)
			if err != nil {
				return fmt.Errorf("finalizing marker: %w", err)
			}
			log.Println("finalizing marker ID", m.ID)
		}

		if _, err := tx.Exec("delete from responses where marker_id=?", m.ID); err != nil {
			return fmt.Errorf("syncing marker: %w", err)
		}

		responseURL, err := baseURL.Parse(m.ResponseURL)
		if err != nil {
			return fmt.Errorf("syncing marker: parsing marker %d response URL %q: %w", m.ID, m.ResponseURL, err)
		}

		b, err := get(responseURL.String())
		if err != nil {
			return fmt.Errorf("syncing marker: getting marker %d response: %w", m.ID, err)
		}

		var data struct {
			Rs []markerResponse `json:"marker_response"`
		}
		if err := json.Unmarshal(b, &data); err != nil {
			return fmt.Errorf("syncing marker: unmarshaling marker %d response: %w", m.ID, err)
		}

		for _, r := range data.Rs {
			r.Question = strings.TrimSpace(r.Question)

			answer := bytes.TrimSpace([]byte(r.Answer))
			if len(answer) == 0 || bytes.Equal(answer, []byte("null")) {
				continue
			}

			if r.QuestionType != "FileQuestion" {
				var ans string
				if err := json.Unmarshal(answer, &ans); err != nil {
					return fmt.Errorf("syncing marker: unmarshaling marker %d response: %w", m.ID, err)
				}
				ans = strings.TrimSpace(ans)
				if len(ans) == 0 {
					continue
				}
				answer = []byte(ans)
			}

			_, err := tx.Exec("insert into responses (marker_id, mode, question_type, question, answer) values (?, ?, ?, ?, ?)", m.ID, r.Mode, r.QuestionType, r.Question, answer)
			if err != nil {
				return fmt.Errorf("syncing marker: adding marker %d response: %w", m.ID, err)
			}
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("syncing marker: adding marker %d responses: %w", m.ID, err)
		}

		log.Println("loaded responses for ID", m.ID)
	}

	fmt.Println("loaded", len(markers), "markers")
	return nil
}

type marker struct {
	Address     string
	Category    markerCategory
	CreatedAt   time.Time `json:"created_at"`
	Editable    bool
	ID          int
	Lat, Lng    string
	ResponseURL string `json:"response_url"`
	User        markerUser
}

type markerCategory struct {
	ID   int
	Name string
}

type markerUser struct {
	Login string
}

type markerResponse struct {
	Answer       json.RawMessage
	Mode         string
	Question     string
	QuestionType string `json:"question_type"`
}

func get(u string) ([]byte, error) {
	resp, err := http.Get(u)
	if err != nil {
		fatalf("requesting %q failed: %s", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		fatalf("requesting %q got bad status %d", u, resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fatalf("error reading %q body: %s", u, err)
	}

	return b, nil
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
