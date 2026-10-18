package seed

// Acceptance tests for ticket P0.4 (seed generator v0).
// They describe the contract, not the implementation: write your own unit
// tests next to them (see the test plan in the ticket).

import (
	"bytes"
	"encoding/csv"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"testing"
	"time"
)

var testStart = time.Date(2026, 11, 2, 0, 0, 0, 0, time.UTC)

func generate(t *testing.T, seed uint64, days int) Data {
	t.Helper()
	d, err := Generate(Config{Start: testStart, Seed: seed, Days: days})
	if err != nil {
		t.Fatalf("Generate(seed=%d, days=%d): %v", seed, days, err)
	}
	if len(d.Flights) == 0 || len(d.Airlines) == 0 || len(d.Airports) == 0 {
		t.Fatalf("Generate returned empty data: %d airlines, %d airports, %d flights",
			len(d.Airlines), len(d.Airports), len(d.Flights))
	}
	return d
}

func TestAcceptanceDeterministic(t *testing.T) {
	a := generate(t, 42, 14)
	b := generate(t, 42, 14)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same Config produced different Data: output must not depend on map iteration or time.Now")
	}
	dirA, dirB := t.TempDir(), t.TempDir()
	if err := WriteCSV(dirA, a); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	if err := WriteCSV(dirB, b); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}
	for _, name := range []string{"airlines.csv", "airports.csv", "flights.csv"} {
		x, errX := os.ReadFile(filepath.Join(dirA, name))
		y, errY := os.ReadFile(filepath.Join(dirB, name))
		if errX != nil || errY != nil {
			t.Fatalf("read %s: %v / %v", name, errX, errY)
		}
		if !bytes.Equal(x, y) {
			t.Errorf("%s differs between two runs with the same Config", name)
		}
	}
}

func TestAcceptanceSeedMatters(t *testing.T) {
	a := generate(t, 1, 7)
	b := generate(t, 2, 7)
	if reflect.DeepEqual(a.Flights, b.Flights) {
		t.Fatal("seeds 1 and 2 produced identical flights: the seed is not used")
	}
}

func TestAcceptanceRejectsBadConfig(t *testing.T) {
	prague := time.FixedZone("CET", 3600)
	for _, tc := range []struct {
		name string
		cfg  Config
	}{
		{"zero days", Config{Start: testStart, Seed: 1, Days: 0}},
		{"negative days", Config{Start: testStart, Seed: 1, Days: -3}},
		{"start not at midnight", Config{Start: testStart.Add(90 * time.Minute), Seed: 1, Days: 1}},
		{"start not in UTC", Config{Start: time.Date(2026, 11, 2, 0, 0, 0, 0, prague), Seed: 1, Days: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Generate(tc.cfg); err == nil {
				t.Errorf("Generate(%+v) returned nil error", tc.cfg)
			}
		})
	}
}

// Airline names must be fictional. Codes may be anything except the designators
// of major carriers below: the two-character IATA space is almost fully taken,
// so "no real code at all" cannot be tested (discussion #023).
var realAirlineCodes = []string{
	"A3", "AF", "AY", "AZ", "BA", "DY", "EI", "EK", "FR", "IB", "KL", "LH",
	"LO", "LX", "OK", "OS", "QR", "SK", "TK", "TP", "U2", "VY", "W6",
}

func TestAcceptanceAirlines(t *testing.T) {
	d := generate(t, 42, 1)
	if len(d.Airlines) < 3 {
		t.Fatalf("got %d airlines, want at least 3", len(d.Airlines))
	}
	code := regexp.MustCompile(`^[A-Z0-9]{2}$`)
	seen := map[string]bool{}
	for _, a := range d.Airlines {
		switch {
		case !code.MatchString(a.Code):
			t.Errorf("airline code %q: want two characters A-Z or 0-9", a.Code)
		case slices.Contains(realAirlineCodes, a.Code):
			t.Errorf("airline code %q belongs to a real airline", a.Code)
		case seen[a.Code]:
			t.Errorf("airline code %q is duplicated", a.Code)
		case a.Name == "":
			t.Errorf("airline %q has no name", a.Code)
		}
		seen[a.Code] = true
	}
}

func TestAcceptanceAirports(t *testing.T) {
	d := generate(t, 42, 1)
	if len(d.Airports) < 12 {
		t.Fatalf("got %d airports, want at least 12", len(d.Airports))
	}
	iata := regexp.MustCompile(`^[A-Z]{3}$`)
	country := regexp.MustCompile(`^[A-Z]{2}$`)
	seen := map[string]bool{}
	for _, a := range d.Airports {
		if !iata.MatchString(a.IATA) || seen[a.IATA] || a.Name == "" || !country.MatchString(a.Country) {
			t.Errorf("bad or duplicated airport %+v", a)
		}
		seen[a.IATA] = true
	}
}

func TestAcceptanceFlights(t *testing.T) {
	const days = 7
	d := generate(t, 42, days)
	airlines, airports := map[string]bool{}, map[string]bool{}
	for _, a := range d.Airlines {
		airlines[a.Code] = true
	}
	for _, a := range d.Airports {
		airports[a.IATA] = true
	}
	end := testStart.AddDate(0, 0, days)
	keyRe := regexp.MustCompile(`^[A-Z0-9]{2}-[1-9][0-9]{0,3}-\d{4}-\d{2}-\d{2}$`)
	keys := map[string]bool{}
	for _, f := range d.Flights {
		dur := f.ArrTime.Sub(f.DepTime)
		switch {
		case !airlines[f.Airline]:
			t.Errorf("%s: unknown airline %q", f.Key(), f.Airline)
		case !airports[f.Dep] || !airports[f.Arr]:
			t.Errorf("%s: unknown airport %s or %s", f.Key(), f.Dep, f.Arr)
		case f.Dep == f.Arr:
			t.Errorf("%s: departs and arrives at %s", f.Key(), f.Dep)
		case f.Number < 1 || f.Number > 9999:
			t.Errorf("%s: flight number %d, want 1..9999", f.Key(), f.Number)
		case f.DepTime.Location() != time.UTC || f.ArrTime.Location() != time.UTC:
			t.Errorf("%s: times must be in UTC", f.Key())
		case f.DepTime.Before(testStart) || !f.DepTime.Before(end):
			t.Errorf("%s: departs %s, outside [%s, %s)", f.Key(), f.DepTime, testStart, end)
		case dur < 45*time.Minute || dur > 5*time.Hour:
			t.Errorf("%s: block time %s, want 45m..5h", f.Key(), dur)
		case !keyRe.MatchString(f.Key()):
			t.Errorf("Key() = %q, want AIRLINE-NUMBER-YYYY-MM-DD", f.Key())
		case f.Key() != f.Airline+"-"+strconv.Itoa(f.Number)+"-"+f.DepTime.Format(time.DateOnly):
			t.Errorf("Key() = %q does not match airline, number and UTC departure date", f.Key())
		case keys[f.Key()]:
			t.Errorf("flight key %s is duplicated", f.Key())
		}
		keys[f.Key()] = true
	}
}

func TestAcceptanceVolume(t *testing.T) {
	week := generate(t, 42, 7)
	perDay := len(week.Flights) / 7
	if perDay < 100 || perDay > 2000 {
		t.Errorf("%d flights per day, want 100..2000", perDay)
	}
	month := generate(t, 42, 28)
	if ratio := float64(len(month.Flights)) / float64(len(week.Flights)); ratio < 3.6 || ratio > 4.4 {
		t.Errorf("28 days give %.2f× the flights of 7 days, want about 4×", ratio)
	}
}

// Search needs itineraries that exist only with a connection: no direct flight
// from A to C on that date, but A→B and then B→C with 60 min .. 12 h to connect.
func TestAcceptanceOneConnectionOnly(t *testing.T) {
	const days = 3
	d := generate(t, 42, days)
	type od struct{ from, to, date string }
	direct := map[od]bool{}
	byDep := map[string][]Flight{}
	for _, f := range d.Flights {
		direct[od{f.Dep, f.Arr, f.DepTime.Format(time.DateOnly)}] = true
		byDep[f.Dep] = append(byDep[f.Dep], f)
	}
	viaOnly := map[od]bool{}
	for _, first := range d.Flights {
		for _, second := range byDep[first.Arr] {
			gap := second.DepTime.Sub(first.ArrTime)
			if gap < time.Hour || gap > 12*time.Hour || second.Arr == first.Dep {
				continue
			}
			k := od{first.Dep, second.Arr, first.DepTime.Format(time.DateOnly)}
			if !direct[k] {
				viaOnly[k] = true
			}
		}
	}
	if got, want := len(viaOnly), 20*days; got < want {
		t.Errorf("%d origin-destination-date triples reachable only with one connection, want at least %d", got, want)
	}
}

func readCSV(t *testing.T, path string) [][]string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer func() { _ = f.Close() }()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	if len(rows) == 0 {
		t.Fatalf("%s is empty, want a header row", path)
	}
	return rows
}

func TestAcceptanceWriteCSV(t *testing.T) {
	d := generate(t, 42, 2)
	dir := t.TempDir()
	if err := WriteCSV(dir, d); err != nil {
		t.Fatalf("WriteCSV: %v", err)
	}

	for _, tc := range []struct {
		file   string
		header []string
		rows   int
	}{
		{"airlines.csv", []string{"code", "name"}, len(d.Airlines)},
		{"airports.csv", []string{"iata", "name", "country"}, len(d.Airports)},
		{"flights.csv", []string{"flight_key", "airline_code", "flight_number", "dep_airport", "arr_airport", "dep_time_utc", "arr_time_utc"}, len(d.Flights)},
	} {
		rows := readCSV(t, filepath.Join(dir, tc.file))
		if !slices.Equal(rows[0], tc.header) {
			t.Errorf("%s header = %q, want %q", tc.file, rows[0], tc.header)
		}
		if got := len(rows) - 1; got != tc.rows {
			t.Errorf("%s has %d data rows, want %d", tc.file, got, tc.rows)
		}
	}

	rows := readCSV(t, filepath.Join(dir, "flights.csv"))[1:]
	if len(rows) != len(d.Flights) {
		t.Fatalf("flights.csv has %d rows, Data has %d flights", len(rows), len(d.Flights))
	}
	var prevDep time.Time
	prevKey := ""
	for i, row := range rows {
		f := d.Flights[i]
		dep, errDep := time.Parse(time.RFC3339, row[5])
		arr, errArr := time.Parse(time.RFC3339, row[6])
		switch {
		case errDep != nil || errArr != nil:
			t.Fatalf("row %d: times %q, %q are not RFC 3339: %v %v", i+1, row[5], row[6], errDep, errArr)
		case row[5][len(row[5])-1] != 'Z' || row[6][len(row[6])-1] != 'Z':
			t.Fatalf("row %d: times must end with Z (UTC), got %q, %q", i+1, row[5], row[6])
		case row[0] != f.Key() || row[1] != f.Airline || row[2] != strconv.Itoa(f.Number) ||
			row[3] != f.Dep || row[4] != f.Arr || !dep.Equal(f.DepTime) || !arr.Equal(f.ArrTime):
			t.Fatalf("row %d = %q does not match Data.Flights[%d] = %+v", i+1, row, i, f)
		case dep.Before(prevDep) || (dep.Equal(prevDep) && row[0] < prevKey):
			t.Fatalf("row %d: flights must be sorted by dep_time_utc, then flight_key", i+1)
		}
		prevDep, prevKey = dep, row[0]
	}
}

func TestAcceptanceWriteCSVBadDir(t *testing.T) {
	notADir := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(notADir, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	err := WriteCSV(notADir, generate(t, 42, 1))
	if err == nil {
		t.Fatal("WriteCSV into a regular file returned nil error")
	}
	var pathErr *os.PathError
	if !errors.As(err, &pathErr) {
		t.Errorf("WriteCSV error %v: want it to wrap the *os.PathError with %%w", err)
	}
}
