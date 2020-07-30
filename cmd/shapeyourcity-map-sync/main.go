package main

import (
	"context"
	"flag"
	"fmt"
	"os"

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
		rm := rm
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

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
