package shapeyourcity

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
)

func TestMapClientMarkers(t *testing.T) {
	now, err := time.Parse(time.RFC3339, "2020-05-28T15:22:40-03:00")
	if err != nil {
		t.Fatal(err)
	}

	const resp = `
{
  "markers": [
    {
      "address": "1234 S Hi Ln, Halifax, NS, B3K 1N2, Canada",
      "category": {
        "name": "Better Cycling"
      },
      "created_at": "2020-05-28T15:22:40-03:00",
      "edit_link": "",
      "editable": true,
      "id": 5,
      "lat": "44.6501359",
      "lng": "-63.5900193",
      "response_url": "/responses/5.json",
      "user": {
        "login": "userx"
      }
    },
    {
      "address": "5432 South Park St, Halifax, NS, B3K 1N2, Canada",
      "category": {
        "name": "Space to Move"
      },
      "created_at": "2020-05-28T15:22:40-03:00",
      "editable": false,
      "id": 10,
      "lat": "44.642074",
      "lng": "-63.5801552",
      "response_url": "https://elsewhere.test.local/responses/ten.json",
      "user": {
        "login": "usery"
      }
    }
  ]
}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/the-map/markers" && r.URL.Query().Get("filter") == "other_users" {
			w.Write([]byte(resp))
			return
		}

		t.Errorf("unexpected method/path/query: %s %q %s", r.Method, r.URL.Path, r.URL.RawQuery)

		w.WriteHeader(404)
	}))
	defer srv.Close()

	cl, err := NewMapClient(srv.URL + "/the-map")
	if err != nil {
		t.Fatal(err)
	}

	got, err := cl.Markers(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	want := []Marker{
		{
			ID:          5,
			User:        "userx",
			CreatedAt:   now,
			Address:     "1234 S Hi Ln, Halifax, NS, B3K 1N2, Canada",
			Category:    "Better Cycling",
			Latitude:    "44.6501359",
			Longitude:   "-63.5900193",
			URL:         srv.URL + "/the-map/#marker-5",
			ResponseURL: srv.URL + "/responses/5.json",
			Editable:    true,
		},
		{
			ID:          10,
			User:        "usery",
			CreatedAt:   now,
			Address:     "5432 South Park St, Halifax, NS, B3K 1N2, Canada",
			Category:    "Space to Move",
			Latitude:    "44.642074",
			Longitude:   "-63.5801552",
			URL:         srv.URL + "/the-map/#marker-10",
			ResponseURL: "https://elsewhere.test.local/responses/ten.json",
			Editable:    false,
		},
	}
	if d := cmp.Diff(want, got); d != "" {
		t.Errorf("markers mismatch (-want +got):\n%s", d)
	}
}

func TestMapClientFillResponses(t *testing.T) {
	const resp = `
{
  "marker_response": [
    {
      "answer": "more bikes, fewer cars",
      "mode": "comment_mode",
      "question": "What change would you like to see on the street?",
      "question_type": "EssayQuestion"
    },
    {
      "answer": {"url":"http://test.local/file.jpg","filename":"IMG_1337.JPG"},
      "mode": "comment_mode",
      "question": "How would you like the street to look?",
      "question_type": "FileQuestion"
    }
  ]
}
`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/responses/x.json" {
			w.Write([]byte(resp))
			return
		}

		t.Errorf("unexpected method/path/query: %s %q %s", r.Method, r.URL.Path, r.URL.RawQuery)

		w.WriteHeader(404)
	}))
	defer srv.Close()

	cl, err := NewMapClient(srv.URL + "/the-map")
	if err != nil {
		t.Fatal(err)
	}

	marker := &Marker{
		ResponseURL: srv.URL + "/responses/x.json",
	}

	if err := cl.FillResponses(context.Background(), marker); err != nil {
		t.Fatal(err)
	}

	want := []MarkerResponse{
		{
			Mode:         "comment_mode",
			QuestionType: "EssayQuestion",
			Question:     "What change would you like to see on the street?",
			Answer:       []byte("more bikes, fewer cars"),
		},
		{
			Mode:         "comment_mode",
			QuestionType: "FileQuestion",
			Question:     "How would you like the street to look?",
			Answer:       []byte(`{"url":"http://test.local/file.jpg","filename":"IMG_1337.JPG"}`),
		},
	}
	if d := cmp.Diff(want, marker.Responses); d != "" {
		t.Errorf("marker responses mismatch (-want +got):\n%s", d)
	}
}
