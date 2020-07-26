package shapeyourcity

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// A Marker is an entry on a map, usually with associated responses.
type Marker struct {
	ID                  int
	User                string
	CreatedAt           time.Time
	Address             string
	Category            string
	Latitude, Longitude string
	URL                 string
	ResponseURL         string
	Editable            bool

	Responses []MarkerResponse
}

// A MarkerResponse captures a response given when creating a marker on a map.
//
// Answer will be a JSON object if QuestionType is FileQuestion,
// otherwise it will be a string. When it is a string, it may have
// HTML entities escaped.
type MarkerResponse struct {
	Mode         string
	QuestionType string
	Question     string
	Answer       []byte
}

// A MapClient fetches data from a map.
type MapClient struct {
	baseURL *url.URL
}

// NewMapClient returns a new MapClient for the given base URL.
//
// The base URL should be the URL visited when viewing the relevant map, eg
// https://www.shapeyourcityhalifax.ca/mobilityresponse/maps/halifax-mobility-response-streets.
func NewMapClient(baseURL string) (*MapClient, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parsing base URL: %w", err)
	}

	if p := u.Path; len(p) < 1 || p[len(p)-1] != '/' {
		u.Path += "/"
	}

	return &MapClient{baseURL: u}, nil
}

// Markers fetches all markers from the client's map.
//
// The returned Marker's Responses field will not be filled in.
// To fill in a Marker's Responses, see FillResponses.
func (c *MapClient) Markers(ctx context.Context) ([]Marker, error) {
	markersURL, err := c.baseURL.Parse("markers?filter=other_users")
	if err != nil {
		return nil, fmt.Errorf("parsing markers URL: %w", err)
	}

	b, err := get(ctx, markersURL.String())
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

	out := make([]Marker, 0, len(markers))

	for _, m := range markers {
		murl, err := c.baseURL.Parse("#marker-" + strconv.Itoa(m.ID))
		if err != nil {
			return nil, fmt.Errorf("building direct marker %d URL: %w", m.ID, err)
		}

		rurl, err := c.baseURL.Parse(m.ResponseURL)
		if err != nil {
			return nil, fmt.Errorf("building marker %d response URL: %w", m.ID, err)
		}

		out = append(out, Marker{
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

// FillResponses fetches responses for the given marker and fills
// marker.Responses with them.
func (c *MapClient) FillResponses(ctx context.Context, marker *Marker) error {
	marker.Responses = nil

	b, err := get(ctx, marker.ResponseURL)
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

		marker.Responses = append(marker.Responses, MarkerResponse{
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

func get(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, fmt.Errorf("requesting %q failed: %w", u, err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting %q failed: %w", u, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("requesting %q got bad status %d", u, resp.StatusCode)
	}

	b, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading %q body: %w", u, err)
	}

	return b, nil
}
