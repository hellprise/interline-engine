package seed

import (
	"cmp"
	"encoding/csv"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"time"
)

var (
	ErrInvalidDays  = errors.New("seed: days must be greater than zero")
	ErrInvalidStart = errors.New("seed: start time must be 00:00:00 UTC")
)

var defaultAirlines = []Airline{
	{Code: "Q7", Name: "Borealis Air"},
	{Code: "Z4", Name: "Solstice Airways"},
	{Code: "X9", Name: "Meridian Wings"},
	{Code: "V8", Name: "Aether Express"},
	{Code: "K5", Name: "Zephyr Lines"},
}

var defaultAirports = []Airport{
	// First 3 airports act as hubs:
	{IATA: "PRG", Name: "Prague Vaclav Havel", Country: "CZ"},
	{IATA: "FRA", Name: "Frankfurt Airport", Country: "DE"},
	{IATA: "VIE", Name: "Vienna International", Country: "AT"},
	// Remaining 11 airports act as spokes:
	{IATA: "MUC", Name: "Munich Airport", Country: "DE"},
	{IATA: "ZRH", Name: "Zurich Airport", Country: "CH"},
	{IATA: "AMS", Name: "Amsterdam Schiphol", Country: "NL"},
	{IATA: "CDG", Name: "Paris Charles de Gaulle", Country: "FR"},
	{IATA: "LHR", Name: "London Heathrow", Country: "GB"},
	{IATA: "MAD", Name: "Madrid Barajas", Country: "ES"},
	{IATA: "BCN", Name: "Barcelona El Prat", Country: "ES"},
	{IATA: "LIS", Name: "Lisbon, Humberto Delgado", Country: "PT"},
	{IATA: "FCO", Name: "Rome Fiumicino", Country: "IT"},
	{IATA: "WAW", Name: "Warsaw Chopin", Country: "PL"},
	{IATA: "CPH", Name: "Copenhagen Airport", Country: "DK"},
}

// Number of repetitions per (hub, spoke) pair per day; each repetition generates both a morning and an afternoon wave (33 * 2 * 2 = 132 flights/day).
const defaultWaves = 2

type Config struct {
	Start time.Time // first service day, 00:00 UTC
	Seed  uint64
	Days  int
}

type Airline struct{ Code, Name string }          // fictional
type Airport struct{ IATA, Name, Country string } // real IATA code, ISO country

type Flight struct {
	DepTime, ArrTime  time.Time // UTC
	Airline, Dep, Arr string    // airline code, IATA, IATA
	Number            int       // 1..9999
}

type flightWindow struct {
	depStartMin int
	depSpanMin  int
	durMin      int
	durSpanMin  int
}

var (
	// Morning wave: spoke -> hub (departs 05:00...08:59 UTC, duration 45...180 min)
	morningWave = flightWindow{depStartMin: 5 * 60, depSpanMin: 4 * 60, durMin: 45, durSpanMin: 136}
	// Afternoon wave: hub -> spoke (departs 13:00...18:59 UTC, duration 45...300 min)
	afternoonWave = flightWindow{depStartMin: 13 * 60, depSpanMin: 6 * 60, durMin: 45, durSpanMin: 256}
)

func (f Flight) Key() string {
	return f.Airline + "-" + strconv.Itoa(f.Number) + "-" + f.DepTime.UTC().Format(time.DateOnly)
}

type Data struct {
	Airlines []Airline
	Airports []Airport
	Flights  []Flight
}

func Generate(cfg Config) (Data, error) {
	if err := validateConfig(cfg); err != nil {
		return Data{}, err
	}

	r := rand.New(rand.NewPCG(cfg.Seed, cfg.Seed^0x9e3779b97f4a7c15))

	airlines := slices.Clone(defaultAirlines)
	airports := slices.Clone(defaultAirports)
	hubs := airports[:3]
	spokes := airports[3:]

	var flights []Flight

	for day := range cfg.Days {
		dayStart := cfg.Start.AddDate(0, 0, day)

		flightNum := 100 + r.IntN(50)

		for _, hub := range hubs {
			for _, spoke := range spokes {
				for range defaultWaves {

					flights = append(flights, randomFlight(r, dayStart, morningWave, airlines, flightNum, spoke.IATA, hub.IATA))
					flightNum++

					flights = append(flights, randomFlight(r, dayStart, afternoonWave, airlines, flightNum, hub.IATA, spoke.IATA))
					flightNum++
				}
			}
		}
	}

	sortFlights(flights)

	return Data{
		Airlines: airlines,
		Airports: airports,
		Flights:  flights,
	}, nil
}

func randomFlight(r *rand.Rand, dayStart time.Time, fw flightWindow, airlines []Airline, number int, dep, arr string) Flight {
	depOffset := time.Duration(fw.depStartMin+r.IntN(fw.depSpanMin)) * time.Minute
	duration := time.Duration(fw.durMin+r.IntN(fw.durSpanMin)) * time.Minute
	depTime := dayStart.Add(depOffset)

	return Flight{
		Airline: airlines[r.IntN(len(airlines))].Code,
		Number:  number,
		Dep:     dep,
		Arr:     arr,
		DepTime: depTime,
		ArrTime: depTime.Add(duration),
	}
}

func WriteCSV(dir string, d Data) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("seed: create dir: %w", err)
	}

	if err := airlineRows(dir, d.Airlines); err != nil {
		return err
	}
	if err := airportRows(dir, d.Airports); err != nil {
		return err
	}
	return flightRows(dir, d.Flights)
}

func writeCSVFile(path string, rows [][]string) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("seed: create %s: %w", path, err)
	}
	defer func() {
		err = errors.Join(err, f.Close())
	}()

	w := csv.NewWriter(f)
	if err := w.WriteAll(rows); err != nil {
		return fmt.Errorf("seed: write %s: %w", path, err)
	}

	return nil
}

// private methods

func validateConfig(cfg Config) error {
	if cfg.Days <= 0 {
		return ErrInvalidDays
	}

	if cfg.Start.Location() != time.UTC || cfg.Start.Hour() != 0 || cfg.Start.Minute() != 0 || cfg.Start.Second() != 0 || cfg.Start.Nanosecond() != 0 {
		return ErrInvalidStart
	}

	return nil
}

func sortFlights(flights []Flight) {
	slices.SortFunc(flights, func(a, b Flight) int {
		if c := a.DepTime.Compare(b.DepTime); c != 0 {
			return c
		}
		return cmp.Compare(a.Key(), b.Key())
	})
}

func airlineRows(dir string, airlines []Airline) error {
	rows := generateRows(
		[]string{"code", "name"},
		airlines,
		func(a Airline) []string { return []string{a.Code, a.Name} },
	)

	if err := writeCSVFile(filepath.Join(dir, "airlines.csv"), rows); err != nil {
		return err
	}

	return nil
}

func airportRows(dir string, airports []Airport) error {
	rows := generateRows(
		[]string{"iata", "name", "country"},
		airports,
		func(a Airport) []string { return []string{a.IATA, a.Name, a.Country} },
	)

	if err := writeCSVFile(filepath.Join(dir, "airports.csv"), rows); err != nil {
		return err
	}

	return nil
}

func flightRows(dir string, flights []Flight) error {
	rows := generateRows(
		[]string{
			"flight_key",
			"airline_code",
			"flight_number",
			"dep_airport",
			"arr_airport",
			"dep_time_utc",
			"arr_time_utc",
		},
		flights,
		func(f Flight) []string {
			return []string{
				f.Key(),
				f.Airline,
				strconv.Itoa(f.Number),
				f.Dep,
				f.Arr,
				f.DepTime.UTC().Format(time.RFC3339),
				f.ArrTime.UTC().Format(time.RFC3339),
			}
		},
	)

	return writeCSVFile(filepath.Join(dir, "flights.csv"), rows)
}

func generateRows[T any](headers []string, input []T, transform func(T) []string) [][]string {
	out := make([][]string, 0, len(input)+1)
	out = append(out, headers)
	for _, item := range input {
		out = append(out, transform(item))
	}
	return out
}
