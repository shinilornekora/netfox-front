package catalog_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"plenka/catalog/internal/adapter/pgsql"
	"plenka/catalog/internal/core"
	"plenka/catalog/internal/usecase"
	"plenka/catalog/migrations"
	"plenka/contracts/events"
	"plenka/kit/pgtest"
	"plenka/kit/principal"
	"plenka/kit/problem"
)

var admin = principal.With(context.Background(), principal.Principal{UserID: uuid.NewString(), Role: principal.Admin})

func film(name string, year int) core.Fields {
	return core.Fields{Kind: core.Film, Name: name, Year: year, AgeRating: 12, Genres: []string{"драма"}, DurationSec: 6000,
		InSubscription: true, Description: "Достаточно длинное описание фильма для публикации."}
}

func TestLifecycleAndEvents(t *testing.T) {
	pools := pgtest.FreshPools(t, "catalog_test", migrations.FS, migrations.Deny...)
	db, owner := pools.App, pools.Owner // outbox читает только владелец (ретранслятор)
	uc := &usecase.Service{Store: pgsql.Store{DB: db}}
	ctx := context.Background()

	viewer := principal.With(ctx, principal.Principal{UserID: uuid.NewString(), Role: principal.Viewer})
	if _, err := uc.Create(viewer, film("X", 2020)); !problem.Is(err, "forbidden") {
		t.Fatalf("viewer cannot create: %v", err)
	}
	f := film("Брат", 1997)
	f.Description = "коротко"
	d, err := uc.Create(admin, f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Publish(admin, d.ID, 0); !problem.Is(err, "invalid") {
		t.Fatalf("short description blocks publishing: %v", err)
	}
	if _, err := uc.Title(ctx, d.ID); !problem.Is(err, "not_found") {
		t.Fatal("draft is hidden from the public")
	}
	d, err = uc.Edit(admin, d.ID, film("Брат", 1997), d.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Edit(admin, d.ID, film("Брат", 1997), 1); !problem.Is(err, "stale") {
		t.Fatalf("stale revision: %v", err)
	}
	p, err := uc.Publish(admin, d.ID, d.Revision)
	if err != nil || p.Status != core.Published || p.PublishedAt == nil {
		t.Fatalf("publish: %+v %v", p, err)
	}
	if err := uc.Delete(admin, p.ID); !problem.Is(err, "not_draft") {
		t.Fatalf("published title cannot be deleted: %v", err)
	}
	if _, err := uc.Withdraw(admin, p.ID, "", 0); !problem.Is(err, "invalid") {
		t.Fatal("withdraw needs a reason")
	}
	w, err := uc.Withdraw(admin, p.ID, "Истекла лицензия", 0)
	if err != nil || w.Status != core.Withdrawn {
		t.Fatal(err)
	}
	if _, err := uc.Publish(admin, w.ID, 0); err != nil {
		t.Fatalf("republish: %v", err)
	}

	// События: черновик — без событий; publish → withdraw → publish.
	rows, _ := owner.Query(ctx, `SELECT body FROM outbox ORDER BY seq`)
	var types []string
	var revs []int64
	for rows.Next() {
		var raw []byte
		_ = rows.Scan(&raw)
		var env events.Envelope
		_ = json.Unmarshal(raw, &env)
		types = append(types, env.Type)
		revs = append(revs, env.Revision)
	}
	rows.Close()
	want := []string{events.TitlePublished, events.TitleWithdrawn, events.TitlePublished}
	if fmt.Sprint(types) != fmt.Sprint(want) {
		t.Fatalf("events %v, want %v", types, want)
	}
	if !(revs[0] < revs[1] && revs[1] < revs[2]) {
		t.Fatalf("revisions must grow: %v", revs)
	}
}

func TestSeriesNeedsEpisodes(t *testing.T) {
	pools := pgtest.FreshPools(t, "catalog_test", migrations.FS, migrations.Deny...)
	db := pools.App
	uc := &usecase.Service{Store: pgsql.Store{DB: db}}
	f := core.Fields{Kind: core.Series, Name: "Тайны следствия", Year: 2000, AgeRating: 16, InSubscription: true,
		Description: "Детективный сериал о работе следователя прокуратуры."}
	s, err := uc.Create(admin, f)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.Publish(admin, s.ID, 0); !problem.Is(err, "invalid") {
		t.Fatal("series without episodes cannot be published")
	}
	f.PriceKopecks = 9900
	if _, err := uc.Edit(admin, s.ID, f, 0); !problem.Is(err, "invalid") {
		t.Fatal("series is subscription-only")
	}
	s, err = uc.SaveEpisode(admin, s.ID, core.Episode{Season: 1, Number: 1, Name: "Пилот", DurationSec: 2700})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.SaveEpisode(admin, s.ID, core.Episode{Season: 1, Number: 1, Name: "Дубль", DurationSec: 2700}); !problem.Is(err, "episode_exists") {
		t.Fatalf("duplicate episode position: %v", err)
	}
	s, err = uc.Publish(admin, s.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uc.DeleteEpisode(admin, s.ID, s.Episodes[0].ID); !problem.Is(err, "last_episode") {
		t.Fatalf("cannot remove the last episode of a published series: %v", err)
	}
}

// Курсорная пагинация: проходим 30 тайтлов страницами по 7; посреди
// прохода публикуется новый тайтл — уже показанные не сдвигаются, нет ни
// повторов, ни пропусков.
func TestKeysetPaginationIsStable(t *testing.T) {
	pools := pgtest.FreshPools(t, "catalog_test", migrations.FS, migrations.Deny...)
	db := pools.App
	uc := &usecase.Service{Store: pgsql.Store{DB: db}}
	for i := 0; i < 30; i++ {
		x, err := uc.Create(admin, film(fmt.Sprintf("Фильм %02d", i), 1990+i%20))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := uc.Publish(admin, x.ID, 0); err != nil {
			t.Fatal(err)
		}
	}
	for _, sort := range []string{"new", "year", "name"} {
		seen := map[uuid.UUID]bool{}
		cursor := ""
		for page := 0; ; page++ {
			p, err := uc.Browse(context.Background(), usecase.Filter{Sort: sort, Limit: 7, Cursor: cursor, AgeMax: -1})
			if err != nil {
				t.Fatal(err)
			}
			for _, it := range p.Items {
				if seen[it.ID] {
					t.Fatalf("%s: duplicate %s", sort, it.Name)
				}
				seen[it.ID] = true
			}
			if page == 1 && sort == "new" {
				x, _ := uc.Create(admin, film("Новинка", 2024))
				_, _ = uc.Publish(admin, x.ID, 0)
			}
			if p.Next == "" {
				break
			}
			cursor = p.Next
		}
		if len(seen) < 30 {
			t.Fatalf("%s: saw %d of 30", sort, len(seen))
		}
	}
}

func TestRussianFullTextSearch(t *testing.T) {
	pools := pgtest.FreshPools(t, "catalog_test", migrations.FS, migrations.Deny...)
	db := pools.App
	uc := &usecase.Service{Store: pgsql.Store{DB: db}}
	f := film("Пираты Карибского моря", 2003)
	f.OriginalName = "Pirates of the Caribbean"
	f.Genres = []string{"приключения", "фэнтези"}
	x, _ := uc.Create(admin, f)
	_, _ = uc.Publish(admin, x.ID, 0)
	y, _ := uc.Create(admin, film("Москва слезам не верит", 1979))
	_, _ = uc.Publish(admin, y.ID, 0)
	for q, want := range map[string]int{"пират": 1, "pirates": 1, "моря": 1, "слеза": 1, "100%": 0} {
		p, err := uc.Browse(context.Background(), usecase.Filter{Query: q, AgeMax: -1})
		if err != nil || len(p.Items) != want {
			t.Fatalf("q=%q: got %d, want %d (%v)", q, len(p.Items), want, err)
		}
	}
	p, _ := uc.Browse(context.Background(), usecase.Filter{Genre: "фэнтези", AgeMax: -1})
	if len(p.Items) != 1 {
		t.Fatal("genre filter")
	}
}

func TestPosterUploadVersionsAndEvent(t *testing.T) {
	pools := pgtest.FreshPools(t, "catalog_test", migrations.FS, migrations.Deny...)
	db, owner := pools.App, pools.Owner // outbox читает только владелец (ретранслятор)
	uc := &usecase.Service{Store: pgsql.Store{DB: db}}
	ctx := context.Background()
	d, _ := uc.Create(admin, film("С постером", 2001))
	png := []byte("\x89PNG\r\n\x1a\n" + string(make([]byte, 64)))
	if _, err := uc.SetPoster(admin, d.ID, "text/plain; charset=utf-8", []byte("not an image")); !problem.Is(err, "invalid") {
		t.Fatalf("non-image rejected: %v", err)
	}
	viewer := principal.With(ctx, principal.Principal{UserID: uuid.NewString(), Role: principal.Viewer})
	if _, err := uc.SetPoster(viewer, d.ID, "image/png", png); !problem.Is(err, "forbidden") {
		t.Fatalf("viewer cannot upload: %v", err)
	}
	d, err := uc.SetPoster(admin, d.ID, "image/png", png)
	if err != nil || d.PosterRev != d.Revision {
		t.Fatalf("poster rev follows revision: %+v %v", d, err)
	}
	if _, _, err := uc.Poster(ctx, d.ID); !problem.Is(err, "not_found") {
		t.Fatal("draft poster hidden from public")
	}
	p, err := uc.Publish(admin, d.ID, d.Revision)
	if err != nil {
		t.Fatal(err)
	}
	ct, data, err := uc.Poster(ctx, d.ID)
	if err != nil || ct != "image/png" || len(data) != len(png) {
		t.Fatalf("public poster: %s %d %v", ct, len(data), err)
	}
	var body []byte
	if err := owner.QueryRow(ctx, `SELECT body FROM outbox ORDER BY seq DESC LIMIT 1`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	var env events.Envelope
	_ = json.Unmarshal(body, &env)
	var snap events.TitleSnapshot
	_ = json.Unmarshal(env.Data, &snap)
	if snap.PosterRev != d.PosterRev {
		t.Fatalf("event carries poster_rev: %d vs %d", snap.PosterRev, d.PosterRev)
	}
	p, _ = uc.DeletePoster(admin, p.ID)
	if p.PosterRev != 0 {
		t.Fatal("poster removed")
	}
}
