// Приёмочный тест тикета Go T1.4: репозиторий мест на pgx.
//
// Скопируй файл в interline-engine/internal/inventory/postgres/. Не меняй его.
// Запуск — нужен Postgres из compose (P1.1):
//
//	ENGINE_DATABASE_URL="postgres://engine:<пароль>@127.0.0.1:5433/engine?sslmode=disable" \
//	  go test ./internal/inventory/... -run Acceptance -v
//
// Без ENGINE_DATABASE_URL тесты с базой пропускаются, проверка пакета inventory работает всегда.
// Каждый тест — в своей схеме t14_acceptance: твои миграции, свои строки, в конце схема удаляется.
package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"

	"github.com/hellprise/interline-engine/internal/inventory"
	"github.com/hellprise/interline-engine/internal/inventory/postgres"
	"github.com/hellprise/interline-engine/migrations"
)

const testSchema = "t14_acceptance"

// Данные каждого теста. Порядок вставки нарочно не по алфавиту.
const seedSQL = `
INSERT INTO seat_inventory (flight_key, cabin, seats_total, seats_held) VALUES
	('V8-220-2026-11-02', 'economy',  150, 148),
	('V8-220-2026-11-02', 'business',  20,   0),
	('V8-220-2026-11-02', 'first',      4,   4),
	('K5-136-2026-11-02', 'economy',   90,  10)`

// seedHeld — seats_held каждой строки из seedSQL.
func seedHeld() map[string]int {
	return map[string]int{
		"V8-220-2026-11-02/economy":  148,
		"V8-220-2026-11-02/business": 0,
		"V8-220-2026-11-02/first":    4,
		"K5-136-2026-11-02/economy":  10,
	}
}

// openRepo создаёт схему testSchema, применяет в ней твои миграции, кладёт строки seedSQL
// и возвращает репозиторий на пуле, который видит только эту схему.
func openRepo(t *testing.T) (*postgres.Repository, *pgxpool.Pool) {
	t.Helper()

	dsn := os.Getenv("ENGINE_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENGINE_DATABASE_URL не задан: подними базу (docker compose up -d) и задай переменную")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("ENGINE_DATABASE_URL: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE"); err != nil {
			t.Errorf("удалить схему %s: %v", testSchema, err)
		}
		admin.Close()
	})
	if _, err := admin.Exec(ctx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE"); err != nil {
		t.Fatalf("не удалось подключиться или удалить схему %s: %v — база запущена?", testSchema, err)
	}
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+testSchema); err != nil {
		t.Fatalf("CREATE SCHEMA %s: %v", testSchema, err)
	}

	// search_path — параметр сеанса: каждое соединение пула работает в testSchema.
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("pgxpool.ParseConfig: %v", err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = testSchema
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("pgxpool.NewWithConfig: %v", err)
	}
	t.Cleanup(pool.Close)

	// goose работает через database/sql — stdlib даёт *sql.DB поверх того же пула.
	db := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { _ = db.Close() })
	p, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS)
	if err != nil {
		t.Fatalf("goose.NewProvider: %v — migrations/embed.go из P1.1 на месте?", err)
	}
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("goose up: %v", err)
	}

	if _, err := pool.Exec(ctx, seedSQL); err != nil {
		t.Fatalf("не удалось положить тестовые строки: %v — у seat_inventory есть обязательные столбцы без умолчания?", err)
	}
	return postgres.NewRepository(pool), pool
}

// heldNow читает seats_held всех строк таблицы: ключ — "рейс/класс".
func heldNow(t *testing.T, pool *pgxpool.Pool) map[string]int {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := pool.Query(ctx, "SELECT flight_key, cabin, seats_held FROM seat_inventory")
	if err != nil {
		t.Fatalf("прочитать seat_inventory: %v", err)
	}
	defer rows.Close()

	got := map[string]int{}
	for rows.Next() {
		var flight, cabin string
		var held int
		if err := rows.Scan(&flight, &cabin, &held); err != nil {
			t.Fatalf("прочитать seat_inventory: %v", err)
		}
		got[flight+"/"+cabin] = held
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("прочитать seat_inventory: %v", err)
	}
	return got
}

// Места рейса: все классы этого рейса, по алфавиту, с верными числами.
func TestAcceptanceFlightSeats(t *testing.T) {
	repo, _ := openRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	got, err := repo.FlightSeats(ctx, "V8-220-2026-11-02")
	if err != nil {
		t.Fatalf("FlightSeats: %v", err)
	}
	want := []inventory.CabinSeats{
		{Cabin: "business", Total: 20, Held: 0},
		{Cabin: "economy", Total: 150, Held: 148},
		{Cabin: "first", Total: 4, Held: 4},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("FlightSeats(V8-220-2026-11-02):\n got  %+v\n want %+v\n(только этот рейс, классы по алфавиту)", got, want)
	}

	wantAvailable := []int{20, 2, 0}
	for i, c := range got {
		if c.Available() != wantAvailable[i] {
			t.Errorf("%s: Available() = %d, ждали %d", c.Cabin, c.Available(), wantAvailable[i])
		}
	}
}

// Нет рейса — доменная ошибка. Ключ рейса — только значение, не кусок SQL.
func TestAcceptanceFlightSeatsUnknown(t *testing.T) {
	repo, _ := openRepo(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for _, key := range []string{"ZZ-000-2026-11-02", "x' OR '1'='1"} {
		got, err := repo.FlightSeats(ctx, key)
		if !errors.Is(err, inventory.ErrFlightNotFound) {
			t.Errorf("FlightSeats(%q): ошибка %v, места %+v — ждали inventory.ErrFlightNotFound", key, err, got)
		}
	}
}

// errName — имя доменной ошибки для сообщений теста.
func errName(err error) string {
	switch err {
	case inventory.ErrFlightNotFound:
		return "inventory.ErrFlightNotFound"
	case inventory.ErrNotEnoughSeats:
		return "inventory.ErrNotEnoughSeats"
	case inventory.ErrInvalidSeatCount:
		return "inventory.ErrInvalidSeatCount"
	}
	return fmt.Sprint(err)
}

// Удержание: что вернул Hold и что стало с seats_held во всей таблице.
func TestAcceptanceHold(t *testing.T) {
	cases := []struct {
		name          string
		flight, cabin string
		n             int
		wantErr       error // nil — удержание прошло
		wantHeld      int   // seats_held этой строки после вызова
	}{
		{"часть свободных мест", "V8-220-2026-11-02", "business", 3, nil, 3},
		{"ровно все свободные", "V8-220-2026-11-02", "economy", 2, nil, 150},
		{"мест не хватает", "V8-220-2026-11-02", "economy", 3, inventory.ErrNotEnoughSeats, 148},
		{"класс полон", "V8-220-2026-11-02", "first", 1, inventory.ErrNotEnoughSeats, 4},
		{"нет рейса", "ZZ-000-2026-11-02", "economy", 1, inventory.ErrFlightNotFound, 0},
		{"на рейсе нет такого класса", "K5-136-2026-11-02", "business", 1, inventory.ErrFlightNotFound, 0},
		{"ноль мест", "V8-220-2026-11-02", "business", 0, inventory.ErrInvalidSeatCount, 0},
		{"отрицательное число мест", "V8-220-2026-11-02", "economy", -5, inventory.ErrInvalidSeatCount, 148},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo, pool := openRepo(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			err := repo.Hold(ctx, c.flight, c.cabin, c.n)
			switch {
			case c.wantErr == nil && err != nil:
				t.Fatalf("Hold(%s, %s, %d): %v — ждали успех", c.flight, c.cabin, c.n, err)
			case c.wantErr != nil && !errors.Is(err, c.wantErr):
				t.Fatalf("Hold(%s, %s, %d): ошибка %v — ждали %s", c.flight, c.cabin, c.n, err, errName(c.wantErr))
			}

			want := seedHeld()
			key := c.flight + "/" + c.cabin
			if _, ok := want[key]; ok {
				want[key] = c.wantHeld
			}
			if got := heldNow(t, pool); !reflect.DeepEqual(got, want) {
				t.Errorf("seats_held после Hold(%s, %s, %d):\n got  %v\n want %v\n(меняется только строка этого рейса и класса, и только при успехе)",
					c.flight, c.cabin, c.n, got, want)
			}
		})
	}
}

// Отменённый контекст доходит до базы: метод возвращает ошибку контекста и ничего не меняет.
func TestAcceptanceContextCanceled(t *testing.T) {
	repo, pool := openRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := repo.FlightSeats(ctx, "V8-220-2026-11-02"); !errors.Is(err, context.Canceled) {
		t.Errorf("FlightSeats с отменённым контекстом: %v — ждали ошибку, для которой errors.Is(err, context.Canceled)", err)
	}
	if err := repo.Hold(ctx, "V8-220-2026-11-02", "business", 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Hold с отменённым контекстом: %v — ждали ошибку, для которой errors.Is(err, context.Canceled)", err)
	}
	if got := heldNow(t, pool); !reflect.DeepEqual(got, seedHeld()) {
		t.Errorf("после отменённого Hold seats_held изменились: %v", got)
	}
}

// Домен не знает про pgx: пакет inventory не импортирует github.com/jackc/pgx.
func TestAcceptanceDomainHasNoPgx(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("..", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		checked++
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if strings.HasPrefix(p, "github.com/jackc/pgx") {
				t.Errorf("%s импортирует %s: в домене не должно быть pgx — переводи ошибки и строки в пакете postgres", path, p)
			}
		}
	}
	if checked == 0 {
		t.Fatal("в internal/inventory нет .go-файлов: тест лежит не в internal/inventory/postgres/?")
	}
}
