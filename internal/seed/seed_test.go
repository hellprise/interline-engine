package seed

import (
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFlightKeyLateDeparture(t *testing.T) {
	f := Flight{
		Airline: "Q7",
		Number:  123,
		DepTime: time.Date(2026, 11, 2, 23, 55, 0, 0, time.UTC),
		ArrTime: time.Date(2026, 11, 3, 1, 30, 0, 0, time.UTC),
	}

	got := f.Key()
	want := "Q7-123-2026-11-02"

	if got != want {
		t.Errorf("f.Key() = %q, want %q", got, want)
	}
}

func TestWriteCSVAirportComma(t *testing.T) {
	dir := t.TempDir()

	airports := Data{Airports: []Airport{{IATA: "LIS", Name: "Lisbon, Humberto Delgado", Country: "PT"}}}

	if err := WriteCSV(dir, airports); err != nil {
		t.Fatalf("WriteCSV failed: %v", err)
	}

	f, err := os.Open(filepath.Join(dir, "airports.csv"))
	if err != nil {
		t.Fatalf("open csv: %v", err)
	}
	defer func() {
		_ = f.Close()
	}()

	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("read csv: %v", err)
	}

	if len(rows[1]) < 3 {
		t.Fatalf("got %d rows, want at least 3", len(rows))
	}

	if got, want := rows[1][1], "Lisbon, Humberto Delgado"; got != want {
		t.Errorf("title column = %q, want %q", got, want)
	}
}

func TestSortFlightsSameDepTime(t *testing.T) {
	sameTime := time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

	flights := []Flight{
		{Airline: "Z4", Number: 200, ArrTime: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC), DepTime: sameTime},
		{Airline: "Q7", Number: 100, ArrTime: time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC), DepTime: sameTime},
	}

	sortFlights(flights)

	if flights[0].Key() != "Q7-100-2026-11-02" || flights[0].Airline != "Q7" {
		t.Errorf("wrong order after sorting: got [%s, %s], want [Q7, Z4]", flights[0].Airline, flights[1].Airline)
	}
}

func TestGenerateSingleDay(t *testing.T) {
	cfg, err := Generate(Config{Start: testStart, Seed: 42, Days: 1})
	if err != nil {
		t.Errorf("generate config failed: %s", err)
	}

	for _, f := range cfg.Flights {
		end := testStart.AddDate(0, 0, 1)

		if f.DepTime.Before(testStart) || !f.DepTime.Before(end) {
			t.Errorf("wrong flight departure time: %s", f.DepTime)
		}
	}
}

func TestGenerateErrorsIs(t *testing.T) {
	cases := []struct {
		name    string
		start   time.Time
		days    int
		wantErr error
	}{
		{name: "invalid days count", start: testStart, days: 0, wantErr: ErrInvalidDays},
		{name: "invalid start time", start: testStart.Add(time.Hour), days: 1, wantErr: ErrInvalidStart},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := Config{Start: tc.start, Seed: 1, Days: tc.days}
			_, err := Generate(options)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Generate(%v) error = %v, want %v", options, err, tc.wantErr)
			}
		})
	}
}
