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
	origLocal := time.Local
	time.Local = time.FixedZone("UTC+3", 3*60*60)
	defer func() {
		time.Local = origLocal
	}()

	zonePlusTwo := time.FixedZone("UTC+2", 2*60*60)

	f := Flight{
		Airline: "Q7",
		Number:  123,
		DepTime: time.Date(2026, 11, 3, 1, 55, 0, 0, zonePlusTwo),
		ArrTime: time.Date(2026, 11, 3, 3, 30, 0, 0, zonePlusTwo),
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

	if len(rows) < 2 || len(rows[1]) < 3 {
		t.Fatalf("unexpected csv shape: got %d rows", len(rows))
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
	data, err := Generate(Config{Start: testStart, Seed: 42, Days: 1})
	if err != nil {
		t.Fatalf("Generate config failed: %s", err)
	}
	if len(data.Flights) == 0 {
		t.Fatal("Generate returned 0 flights, want > 0")
	}

	end := testStart.AddDate(0, 0, 1)
	for _, f := range data.Flights {
		if f.DepTime.Before(testStart) || !f.DepTime.Before(end) {
			t.Errorf("flight DepTime = %v, want within [%v, %v)", f.DepTime, testStart, end)
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
