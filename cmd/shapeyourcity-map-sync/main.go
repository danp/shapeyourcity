package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danp/shapeyourcity"
	"github.com/danp/shapeyourcity/internal/store"
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

	cl, err := shapeyourcity.NewMapClient(*baseURLA)
	if err != nil {
		fatalf("creating map client: %w", err)
	}

	st, err := store.NewDB(*databaseFile)
	if err != nil {
		fatalf("error initializing store: %w", err)
	}

	err = sync(context.Background(), st, cl)

	st.Close()

	if err != nil {
		fatalf("error syncing: %s", err)
	}
}

func sync(ctx context.Context, st *store.DB, cl *shapeyourcity.MapClient) error {
	storedMarkers, err := st.Markers(ctx)
	if err != nil {
		return fmt.Errorf("gathering stored markers: %w", err)
	}

	storedMarkersByID := make(map[int]shapeyourcity.Marker)
	for _, m := range storedMarkers {
		storedMarkersByID[m.ID] = m
	}

	remoteMarkers, err := cl.Markers(ctx)
	if err != nil {
		return fmt.Errorf("fetching remote markers: %w", err)
	}

	for _, rm := range remoteMarkers {
		sm, known := storedMarkersByID[rm.ID]
		if known && !sm.Editable {
			continue
		}

		if err := cl.FillResponses(ctx, &rm); err != nil {
			return fmt.Errorf("filling marker %d responses: %w", rm.ID, err)
		}

		if err := st.Sync(ctx, rm); err != nil {
			return fmt.Errorf("syncing marker %d: %w", rm.ID, err)
		}

		delete(storedMarkersByID, rm.ID)
	}

	return nil
}

type mapClient struct {
	baseURL *url.URL
}

func (c *mapClient) markers() ([]shapeyourcity.Marker, error) {
	markersURL, err := c.baseURL.Parse("markers?filter=other_users")
	if err != nil {
		return nil, fmt.Errorf("parsing markers URL: %w", err)
	}

	b, err := get(markersURL.String())
	if err != nil {
		return nil, fmt.Errorf("getting markers: %w", err)
	}

	var data struct {
		Markers []clientMarker
	}

	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unmarshaling markers: %w", err)
	}

	markers := data.Markers

	out := make([]shapeyourcity.Marker, 0, len(markers))

	for _, m := range markers {
		murl, err := c.baseURL.Parse("#marker-" + strconv.Itoa(m.ID))
		if err != nil {
			return nil, fmt.Errorf("building direct marker %d URL: %w", m.ID, err)
		}

		rurl, err := c.baseURL.Parse(m.ResponseURL)
		if err != nil {
			return nil, fmt.Errorf("building marker %d response URL: %w", m.ID, err)
		}

		out = append(out, shapeyourcity.Marker{
			ID:          m.ID,
			User:        m.User.Login,
			CreatedAt:   m.CreatedAt,
			Address:     m.Address,
			Category:    m.Category.Name,
			Latitude:    m.Lat,
			Longitude:   m.Lng,
			URL:         murl.String(),
			ResponseURL: rurl.String(),
			Editable:    m.Editable,
		})
	}

	return out, nil
}

func (c *mapClient) fillResponses(marker *shapeyourcity.Marker) error {
	marker.Responses = nil

	b, err := get(marker.ResponseURL)
	if err != nil {
		return fmt.Errorf("getting marker %d responses: %w", marker.ID, err)
	}

	var data struct {
		Rs []clientMarkerResponse `json:"marker_response"`
	}
	if err := json.Unmarshal(b, &data); err != nil {
		return fmt.Errorf("unmarshaling marker %d responses: %w", marker.ID, err)
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
				return fmt.Errorf("unmarshaling marker %d response: %w", marker.ID, err)
			}
			ans = strings.TrimSpace(ans)
			if len(ans) == 0 {
				continue
			}
			answer = []byte(ans)
		}
		r.Answer = answer

		marker.Responses = append(marker.Responses, shapeyourcity.MarkerResponse{
			Mode:         r.Mode,
			QuestionType: r.QuestionType,
			Question:     r.Question,
			Answer:       r.Answer,
		})
	}

	return nil
}

type clientMarker struct {
	Address     string
	Category    clientMarkerCategory
	CreatedAt   time.Time `json:"created_at"`
	Editable    bool
	ID          int
	Lat, Lng    string
	ResponseURL string `json:"response_url"`
	User        clientMarkerUser
	URL         string

	Responses []clientMarkerResponse
}

type clientMarkerCategory struct {
	ID   int
	Name string
}

type clientMarkerUser struct {
	Login string
}

type clientMarkerResponse struct {
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
