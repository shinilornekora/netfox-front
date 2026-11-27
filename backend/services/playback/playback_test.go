package playback_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"plenka/contracts/events"
	"plenka/kit/pgkit"
	"plenka/kit/pgtest"
	"plenka/kit/principal"
	"plenka/kit/problem"
	"plenka/playback/internal/adapter/pgsql"
	"plenka/playback/internal/usecase"
	"plenka/playback/migrations"
)

type fakeAccess struct {
	allowed map[uuid.UUID]bool
	err     error
}

func (f fakeAccess) Check(_ context.Context, _, title uuid.UUID) (bool, string, string, error) {
	if f.err != nil {
		return false, "", "", f.err
	}
	if f.allowed[title] {
		return true, "subscription", "", nil
	}
	return false, "", "not_entitled", nil
}

type env struct {
	db    *pgxpool.Pool // роль данных, как у сервиса
	owner *pgxpool.Pool // владелец схемы: DDL секций
	uc    *usecase.Service
	film  uuid.UUID
	adult uuid.UUID
	show  uuid.UUID
	ep1   uuid.UUID
	ep2   uuid.UUID
}

func setup(t *testing.T) *env {
	pools := pgtest.FreshPools(t, "playback_test", migrations.FS)
	db := pools.App
	e := &env{db: db, owner: pools.Owner, film: uuid.New(), adult: uuid.New(), show: uuid.New(), ep1: uuid.New(), ep2: uuid.New()}
	e.uc = &usecase.Service{Store: pgsql.Store{DB: db}, StreamKey: []byte("k"),
		Access: fakeAccess{allowed: map[uuid.UUID]bool{e.film: true, e.adult: true, e.show: true}}}
	e.apply(t, events.TitleSnapshot{ID: e.film.String(), Kind: "film", Name: "Фильм", Year: 2020, AgeRating: 12, Status: "published", DurationSec: 6000, Revision: 1})
	e.apply(t, events.TitleSnapshot{ID: e.adult.String(), Kind: "film", Name: "Для взрослых", Year: 2020, AgeRating: 18, Status: "published", DurationSec: 6000, Revision: 1})
	e.apply(t, events.TitleSnapshot{ID: e.show.String(), Kind: "series", Name: "Сериал", Year: 2021, AgeRating: 16, Status: "published", Revision: 1,
		Episodes: []events.Episode{{ID: e.ep2.String(), Season: 1, Number: 2, Name: "Вторая", DurationSec: 1800}, {ID: e.ep1.String(), Season: 1, Number: 1, Name: "Первая", DurationSec: 1800}}})
	return e
}

func (e *env) apply(t *testing.T, s events.TitleSnapshot) {
	data, _ := json.Marshal(s)
	if err := e.uc.HandleCatalog(context.Background(), events.Envelope{ID: uuid.NewString(), Type: events.TitleUpdated, Data: data}); err != nil {
		t.Fatal(err)
	}
}

func viewer(id uuid.UUID, birth int) context.Context {
	return principal.With(context.Background(), principal.Principal{UserID: id.String(), Role: principal.Viewer, BirthYear: birth})
}

func TestPlayResumeAndAgeGate(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	ctx := viewer(u, 1995)
	st, err := e.uc.Play(ctx, e.film, nil)
	if err != nil || st.ResumeAtSec != 0 || st.StreamURL == "" {
		t.Fatalf("first play: %+v %v", st, err)
	}
	if _, _, err := e.uc.SaveProgress(ctx, e.film, nil, 1234, 6000, 1000); err != nil {
		t.Fatal(err)
	}
	st, _ = e.uc.Play(ctx, e.film, nil)
	if st.ResumeAtSec != 1229 {
		t.Fatalf("resume 5 seconds before the stop point: %d", st.ResumeAtSec)
	}
	// Подросток и зритель без даты рождения не видят 18+.
	if _, err := e.uc.Play(viewer(uuid.New(), time.Now().Year()-15), e.adult, nil); !problem.Is(err, "forbidden") {
		t.Fatalf("15-year-old and 18+: %v", err)
	}
	if _, err := e.uc.Play(viewer(uuid.New(), 0), e.adult, nil); !problem.Is(err, "forbidden") {
		t.Fatalf("unknown age and 18+: %v", err)
	}
	if _, err := e.uc.Play(viewer(uuid.New(), 0), e.film, nil); err != nil {
		t.Fatalf("12+ with unknown age is allowed: %v", err)
	}
	// Снятый с показа не стартует.
	e.apply(t, events.TitleSnapshot{ID: e.film.String(), Kind: "film", Name: "Фильм", Year: 2020, AgeRating: 12, Status: "withdrawn", DurationSec: 6000, Revision: 2})
	if _, err := e.uc.Play(ctx, e.film, nil); !problem.Is(err, "not_available") {
		t.Fatalf("withdrawn: %v", err)
	}
}

func TestSeriesStartsFromFirstOrLastEpisode(t *testing.T) {
	e := setup(t)
	ctx := viewer(uuid.New(), 1990)
	st, err := e.uc.Play(ctx, e.show, nil)
	if err != nil || st.Episode == nil || st.Episode.ID != e.ep1.String() {
		t.Fatalf("series starts from 1x1: %+v %v", st.Episode, err)
	}
	ep2 := e.ep2
	if _, _, err := e.uc.SaveProgress(ctx, e.show, &ep2, 600, 1800, 1); err != nil {
		t.Fatal(err)
	}
	st, _ = e.uc.Play(ctx, e.show, nil)
	if st.Episode.ID != e.ep2.String() || st.ResumeAtSec != 595 {
		t.Fatalf("continue from the last episode: %+v", st)
	}
}

// Пинги прогресса приходят не по порядку: побеждает более поздний по
// времени устройства, а не по времени прихода.
func TestOutOfOrderProgress(t *testing.T) {
	e := setup(t)
	ctx := viewer(uuid.New(), 1990)
	_, ok, _ := e.uc.SaveProgress(ctx, e.film, nil, 3000, 6000, 2000)
	_, late, _ := e.uc.SaveProgress(ctx, e.film, nil, 100, 6000, 1000)
	if !ok || late {
		t.Fatalf("older ping must be ignored: first=%v late=%v", ok, late)
	}
	p, _, _ := e.uc.ProgressFor(ctx, e.film)
	if p.PositionSec != 3000 {
		t.Fatalf("position %d", p.PositionSec)
	}
	_, _, _ = e.uc.SaveProgress(ctx, e.film, nil, 5700, 6000, 3000)
	list, _, _ := e.uc.Continue(ctx)
	if len(list) != 0 {
		t.Fatal("95% watched — no longer in «continue watching»")
	}
}

func TestRowLevelSecurityAndWatchlist(t *testing.T) {
	e := setup(t)
	alice, bob := uuid.New(), uuid.New()
	if err := e.uc.AddWatch(viewer(alice, 1990), e.film); err != nil {
		t.Fatal(err)
	}
	_ = e.uc.AddWatch(viewer(alice, 1990), e.film) // идемпотентно
	a, _, _ := e.uc.Watchlist(viewer(alice, 1990))
	b, _, _ := e.uc.Watchlist(viewer(bob, 1990))
	if len(a) != 1 || len(b) != 0 {
		t.Fatalf("watchlists: alice %d, bob %d", len(a), len(b))
	}
	ctx := context.Background()
	var n int
	if err := e.db.QueryRow(ctx, `SELECT count(*) FROM watchlist`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("without plenka.user_id RLS must hide all rows")
	}
	// Попытка записать строку за другого пользователя отклоняется WITH CHECK.
	tx, _ := e.db.Begin(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, bob.String()); err != nil {
		t.Fatal(err)
	}
	_, err := tx.Exec(ctx, `INSERT INTO watchlist(user_id, title_id) VALUES ($1, $2)`, alice, e.film)
	_ = tx.Rollback(ctx)
	if c := pgkit.Code(err); c != "42501" { // new row violates row-level security policy
		t.Fatalf("writing a row for another viewer must be refused by WITH CHECK, code=%q err=%v", c, err)
	}
}

func TestHistoryPartitions(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	month := time.Now().UTC().Format("2006_01")
	var exists bool
	if err := e.owner.QueryRow(ctx, `SELECT to_regclass('views_'||$1) IS NOT NULL`, month).Scan(&exists); err != nil {
		t.Fatal(err)
	}
	if !exists {
		t.Fatalf("partition views_%s must exist", month)
	}
	u := uuid.New()
	for i := 0; i < 35; i++ {
		if _, err := e.uc.Play(viewer(u, 1990), e.film, nil); err != nil {
			t.Fatal(err)
		}
	}
	page1, _, err := e.uc.History(viewer(u, 1990), nil)
	if err != nil || len(page1) != 30 {
		t.Fatalf("first page: %d %v", len(page1), err)
	}
	last := page1[len(page1)-1]
	page2, _, _ := e.uc.History(viewer(u, 1990), &usecase.Cursor{At: last.StartedAt, ID: last.ID})
	if len(page2) != 5 {
		t.Fatalf("second page: %d", len(page2))
	}
	// Строки физически лежат в секции текущего месяца, а не в DEFAULT.
	tx, _ := e.owner.Begin(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, u.String()); err != nil {
		t.Fatal(err)
	}
	var inMonth, inDefault int
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM views_%s`, month)).Scan(&inMonth); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM views_default`).Scan(&inDefault); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if inMonth != 35 || inDefault != 0 {
		t.Fatalf("partition routing: month=%d default=%d", inMonth, inDefault)
	}
	if n, _ := (pgsql.Store{DB: e.owner}).EnsurePartitions(ctx); n != 0 {
		t.Fatalf("partition maintenance must be idempotent, created %d", n)
	}

	// Срок хранения: секция двухлетней давности удаляется через DETACH + DROP,
	// текущий месяц и DEFAULT остаются.
	old := time.Now().UTC().AddDate(0, -24, 0)
	from := time.Date(old.Year(), old.Month(), 1, 0, 0, 0, 0, time.UTC)
	oldName := "views_" + from.Format("2006_01")
	if _, err := e.owner.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s PARTITION OF views FOR VALUES FROM ('%s') TO ('%s')`,
		oldName, from.Format("2006-01-02"), from.AddDate(0, 1, 0).Format("2006-01-02"))); err != nil {
		t.Fatal(err)
	}
	// Строка трёхлетней давности (секции на этот месяц нет) попадает в DEFAULT
	// и должна удалиться той же границей.
	ancient := time.Now().UTC().AddDate(-3, 0, 0)
	if err := pgx.BeginFunc(ctx, e.owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, u.String()); err != nil {
			t.Fatal(err)
		}
		_, err := tx.Exec(ctx, `INSERT INTO views(id, user_id, title_id, via, started_at) VALUES ($1,$2,$3,'subscription',$4)`,
			uuid.New(), u, e.film, ancient)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	n, err := (pgsql.Store{DB: e.owner}).DropOldPartitions(ctx, pgsql.HistoryKeepMonths)
	if err != nil || n != 1 {
		t.Fatalf("retention: dropped %d, %v", n, err)
	}
	var oldExists, curExists, defExists bool
	if err := e.owner.QueryRow(ctx, `SELECT to_regclass($1) IS NOT NULL, to_regclass('views_'||$2) IS NOT NULL, to_regclass('views_default') IS NOT NULL`,
		oldName, month).Scan(&oldExists, &curExists, &defExists); err != nil {
		t.Fatal(err)
	}
	if oldExists || !curExists || !defExists {
		t.Fatalf("after retention: old=%v current=%v default=%v", oldExists, curExists, defExists)
	}
	var leftInDefault int
	_ = pgx.BeginFunc(ctx, e.owner, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT set_config('plenka.user_id', $1, true)`, u.String()); err != nil {
			t.Fatal(err)
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM views_default`).Scan(&leftInDefault)
	})
	if leftInDefault != 0 {
		t.Fatalf("old rows left in views_default: %d", leftInDefault)
	}
}

func TestEntitlementAndStreamToken(t *testing.T) {
	e := setup(t)
	u := uuid.New()
	e.uc.Access = fakeAccess{allowed: map[uuid.UUID]bool{}}
	if _, err := e.uc.Play(viewer(u, 1990), e.film, nil); !problem.Is(err, "not_entitled") {
		t.Fatalf("no subscription: %v", err)
	}
	e.uc.Access = fakeAccess{err: errors.New("billing down")}
	if _, err := e.uc.Play(viewer(u, 1990), e.film, nil); !problem.Is(err, "access_unavailable") {
		t.Fatalf("billing down: %v", err)
	}
	e.uc.Access = fakeAccess{allowed: map[uuid.UUID]bool{e.film: true}}
	st, _ := e.uc.Play(viewer(u, 1990), e.film, nil)
	tok := st.StreamURL[len("/api/playback/stream/"):]
	if id, media, err := e.uc.VerifyStream(tok); err != nil || id != e.film || media != usecase.DemoMedia || !st.Demo {
		t.Fatalf("token verify: %v", err)
	}
	if _, _, err := e.uc.VerifyStream(tok[:len(tok)-2] + "xx"); err == nil {
		t.Fatal("tampered token")
	}
	e.uc.Clock = func() time.Time { return time.Now().Add(5 * time.Hour) }
	if _, _, err := e.uc.VerifyStream(tok); !problem.Is(err, "stream_expired") {
		t.Fatal("expired token")
	}
}

func TestFutureClockRejected(t *testing.T) {
	e := setup(t)
	ctx := viewer(uuid.New(), 1990)
	future := time.Now().Add(time.Hour).UnixMilli()
	if _, _, err := e.uc.SaveProgress(ctx, e.film, nil, 100, 6000, future); !problem.Is(err, "invalid") {
		t.Fatalf("clock an hour ahead must be rejected: %v", err)
	}
}

func TestAdminPreviewSkipsBillingAndAge(t *testing.T) {
	e := setup(t)
	e.uc.Access = fakeAccess{err: errors.New("billing must not be called")}
	adm := principal.With(context.Background(), principal.Principal{UserID: uuid.NewString(), Role: principal.Admin})
	st, err := e.uc.Play(adm, e.adult, nil)
	if err != nil || st.Via != "preview" {
		t.Fatalf("admin preview: %+v %v", st, err)
	}
	if err := e.uc.AddWatch(adm, e.film); err != nil {
		t.Fatalf("admin can use watchlist: %v", err)
	}
}

func TestRealMediaChosenPerEpisode(t *testing.T) {
	e := setup(t)
	e.apply(t, events.TitleSnapshot{ID: e.show.String(), Kind: "series", Name: "Сериал", Year: 2021, AgeRating: 16, Status: "published", Revision: 5,
		Episodes: []events.Episode{{ID: e.ep1.String(), Season: 1, Number: 1, Name: "Первая", DurationSec: 1800, Media: "show-01.mp4"},
			{ID: e.ep2.String(), Season: 1, Number: 2, Name: "Вторая", DurationSec: 1800, Media: "show-02.mp4"}}})
	e.uc.HasMedia = func(n string) bool { return n == "show-02.mp4" }
	ctx := viewer(uuid.New(), 1990)
	ep2 := e.ep2
	st, err := e.uc.Play(ctx, e.show, &ep2)
	if err != nil || st.Demo {
		t.Fatalf("episode 2 has its own file: %+v %v", st, err)
	}
	_, media, _ := e.uc.VerifyStream(st.StreamURL[len("/api/playback/stream/"):])
	if media != "show-02.mp4" {
		t.Fatalf("media in token: %s", media)
	}
	st, _ = e.uc.Play(ctx, e.show, &e.ep1)
	if !st.Demo {
		t.Fatal("missing file falls back to demo")
	}
}
