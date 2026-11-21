// Приёмочный тест тикета Go P1.1: таблица seat_inventory и откат миграций.
//
// Скопируй файл в interline-engine/migrations/, рядом с embed.go. Не меняй его.
// Запуск — нужен Postgres из compose:
//
//	ENGINE_DATABASE_URL="postgres://engine:<пароль>@127.0.0.1:5433/engine?sslmode=disable" \
//	  go test ./migrations -run Acceptance -v
//
// Без ENGINE_DATABASE_URL тесты пропускаются: CI без базы остаётся зелёным.
// Тест работает в своей схеме p11_acceptance и удаляет её в конце — твои таблицы не трогает.
package migrations

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	_ "github.com/jackc/pgx/v5/stdlib" // драйвер "pgx" для database/sql
	"github.com/pressly/goose/v3"
)

const testSchema = "p11_acceptance"

// openTestDB подключается к базе из ENGINE_DATABASE_URL так, что все таблицы
// создаются в отдельной схеме testSchema. Схема создаётся заново и удаляется после теста.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	dsn := os.Getenv("ENGINE_DATABASE_URL")
	if dsn == "" {
		t.Skip("ENGINE_DATABASE_URL не задан: подними базу (docker compose up -d) и задай переменную")
	}
	u, err := url.Parse(dsn)
	if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
		t.Fatalf("ENGINE_DATABASE_URL должен быть вида postgres://user:pass@host:port/db?sslmode=disable, получили %q", dsn)
	}

	resetSchema(t, dsn, "CREATE SCHEMA "+testSchema)
	t.Cleanup(func() { resetSchema(t, dsn, "") })

	// search_path — параметр сеанса: pgx передаёт его серверу при подключении,
	// и все таблицы, включая goose_db_version, создаются в testSchema.
	q := u.Query()
	q.Set("search_path", testSchema)
	u.RawQuery = q.Encode()

	db, err := sql.Open("pgx", u.String())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// resetSchema удаляет testSchema и, если then не пустой, выполняет then.
func resetSchema(t *testing.T, dsn, then string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer func() { _ = admin.Close() }()

	if _, err := admin.ExecContext(ctx, "DROP SCHEMA IF EXISTS "+testSchema+" CASCADE"); err != nil {
		t.Fatalf("не удалось подключиться или удалить схему %s: %v — база запущена?", testSchema, err)
	}
	if then == "" {
		return
	}
	if _, err := admin.ExecContext(ctx, then); err != nil {
		t.Fatalf("%s: %v", then, err)
	}
}

func upAll(t *testing.T, db *sql.DB) *goose.Provider {
	t.Helper()
	p, err := goose.NewProvider(goose.DialectPostgres, db, FS)
	if err != nil {
		t.Fatalf("goose.NewProvider: %v — есть ли в migrations/ файлы .sql и embed.go с //go:embed *.sql?", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("goose up: %v", err)
	}
	return p
}

// userTables возвращает таблицы testSchema, кроме служебной goose_db_version.
func userTables(t *testing.T, db *sql.DB) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `
		SELECT table_name FROM information_schema.tables
		WHERE table_schema = $1 AND table_name <> 'goose_db_version'
		ORDER BY table_name`, testSchema)
	if err != nil {
		t.Fatalf("список таблиц: %v", err)
	}
	defer func() { _ = rows.Close() }()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("список таблиц: %v", err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("список таблиц: %v", err)
	}
	return names
}

func contains(names []string, want string) bool {
	for _, n := range names {
		if n == want {
			return true
		}
	}
	return false
}

// up → down до 0 → up: откат убирает всё, что создал Up, и миграции применяются снова.
func TestAcceptanceUpDownUp(t *testing.T) {
	db := openTestDB(t)
	p := upAll(t, db)

	if got := userTables(t, db); !contains(got, "seat_inventory") {
		t.Fatalf("после goose up нет таблицы seat_inventory; таблицы в схеме: %v", got)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := p.DownTo(ctx, 0); err != nil {
		t.Fatalf("goose down до версии 0: %v — у каждой миграции есть -- +goose Down?", err)
	}
	if got := userTables(t, db); len(got) != 0 {
		t.Fatalf("после отката всех миграций остались таблицы %v: Down должен убирать то, что создал Up", got)
	}

	if _, err := p.Up(ctx); err != nil {
		t.Fatalf("повторный goose up после отката: %v", err)
	}
	if got := userTables(t, db); !contains(got, "seat_inventory") {
		t.Fatalf("после повторного goose up нет seat_inventory; таблицы: %v", got)
	}
}

// Четыре обязательных столбца: тип, NOT NULL и умолчание у seats_held.
func TestAcceptanceSeatInventoryColumns(t *testing.T) {
	db := openTestDB(t)
	upAll(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx, `
		SELECT column_name, data_type, is_nullable, coalesce(column_default, '')
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = 'seat_inventory'`, testSchema)
	if err != nil {
		t.Fatalf("столбцы seat_inventory: %v", err)
	}
	defer func() { _ = rows.Close() }()

	type column struct{ dataType, nullable, def string }
	got := map[string]column{}
	for rows.Next() {
		var name string
		var c column
		if err := rows.Scan(&name, &c.dataType, &c.nullable, &c.def); err != nil {
			t.Fatalf("столбцы seat_inventory: %v", err)
		}
		got[name] = c
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("столбцы seat_inventory: %v", err)
	}

	want := map[string]string{
		"flight_key":  "text",
		"cabin":       "text",
		"seats_total": "integer",
		"seats_held":  "integer",
	}
	for name, wantType := range want {
		c, ok := got[name]
		switch {
		case !ok:
			t.Errorf("нет столбца %s", name)
		case c.dataType != wantType:
			t.Errorf("%s: тип %s, ждали %s", name, c.dataType, wantType)
		case c.nullable != "NO":
			t.Errorf("%s: допускает NULL, а он обязателен", name)
		}
	}
	if c, ok := got["seats_held"]; ok && strings.TrimSpace(c.def) != "0" {
		t.Errorf("seats_held: умолчание %q, ждали 0", c.def)
	}
}

// Правила таблицы проверяет сама база: пара «рейс + класс» одна, 0 <= seats_held <= seats_total.
func TestAcceptanceSeatInventoryRules(t *testing.T) {
	db := openTestDB(t)
	upAll(t, db)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	exec := func(query string) error {
		_, err := db.ExecContext(ctx, query)
		return err
	}

	allowed := []struct{ name, query string }{
		{"строка без seats_held", `INSERT INTO seat_inventory (flight_key, cabin, seats_total) VALUES ('V8-220-2026-11-02', 'economy', 150)`},
		{"тот же рейс, другой класс", `INSERT INTO seat_inventory (flight_key, cabin, seats_total) VALUES ('V8-220-2026-11-02', 'business', 20)`},
		{"удержаны все места", `INSERT INTO seat_inventory (flight_key, cabin, seats_total, seats_held) VALUES ('K5-136-2026-11-02', 'economy', 10, 10)`},
	}
	for _, c := range allowed {
		if err := exec(c.query); err != nil {
			t.Errorf("%s: база отвергла допустимую строку: %v", c.name, err)
		}
	}

	var held int
	err := db.QueryRowContext(ctx,
		`SELECT seats_held FROM seat_inventory WHERE flight_key = 'V8-220-2026-11-02' AND cabin = 'economy'`).Scan(&held)
	if err != nil {
		t.Errorf("прочитать seats_held: %v", err)
	} else if held != 0 {
		t.Errorf("seats_held без значения = %d, ждали 0", held)
	}

	// SQLSTATE: 23505 — дубль ключа, 23514 — нарушен CHECK, 23502 — NULL в обязательном столбце.
	rejected := []struct{ name, query, code string }{
		{"вторая строка для той же пары рейс + класс", `INSERT INTO seat_inventory (flight_key, cabin, seats_total) VALUES ('V8-220-2026-11-02', 'economy', 10)`, "23505"},
		{"удержано больше, чем всего", `INSERT INTO seat_inventory (flight_key, cabin, seats_total, seats_held) VALUES ('Z4-205-2026-11-02', 'economy', 10, 11)`, "23514"},
		{"удержано меньше нуля", `INSERT INTO seat_inventory (flight_key, cabin, seats_total, seats_held) VALUES ('Z4-205-2026-11-02', 'business', 10, -1)`, "23514"},
		{"удержание растёт за предел", `UPDATE seat_inventory SET seats_held = seats_held + 1 WHERE flight_key = 'K5-136-2026-11-02' AND cabin = 'economy'`, "23514"},
		{"нет рейса", `INSERT INTO seat_inventory (cabin, seats_total) VALUES ('economy', 10)`, "23502"},
		{"нет класса", `INSERT INTO seat_inventory (flight_key, seats_total) VALUES ('Z4-205-2026-11-02', 10)`, "23502"},
		{"нет числа мест", `INSERT INTO seat_inventory (flight_key, cabin) VALUES ('Z4-205-2026-11-02', 'economy')`, "23502"},
		{"seats_held = NULL", `INSERT INTO seat_inventory (flight_key, cabin, seats_total, seats_held) VALUES ('Z4-205-2026-11-02', 'economy', 10, NULL)`, "23502"},
	}
	for _, c := range rejected {
		err := exec(c.query)
		var pgErr *pgconn.PgError
		switch {
		case err == nil:
			t.Errorf("%s: база приняла строку, а должна была отвергнуть (ждали SQLSTATE %s)", c.name, c.code)
		case !errors.As(err, &pgErr):
			t.Errorf("%s: не ошибка Postgres: %v", c.name, err)
		case pgErr.Code != c.code:
			t.Errorf("%s: SQLSTATE %s (%s), ждали %s", c.name, pgErr.Code, pgErr.Message, c.code)
		}
	}
}
