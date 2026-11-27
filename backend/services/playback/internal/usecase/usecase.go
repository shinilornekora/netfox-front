// Package usecase — старт просмотра, прогресс, история, «Смотреть позже».
package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"plenka/contracts/events"
	"plenka/kit/bus"
	"plenka/kit/principal"
	"plenka/kit/problem"
	"plenka/playback/internal/core"
)

// Access — порт к billing: «может ли зритель смотреть».
type Access interface {
	Check(ctx context.Context, user, title uuid.UUID) (allowed bool, via string, denial string, err error)
}

type Store interface {
	Title(ctx context.Context, id uuid.UUID) (core.Title, bool, error)
	Titles(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]core.Title, error)
	ApplyTitle(ctx context.Context, env events.Envelope, t events.TitleSnapshot) error
	ApplyOrder(ctx context.Context, env events.Envelope, o events.OrderSnapshot) (bool, error)

	// Всё ниже — от имени зрителя (RLS).
	RecordView(ctx context.Context, user uuid.UUID, v core.View) error
	Progress(ctx context.Context, user, title uuid.UUID) (*core.Progress, error)
	SaveProgress(ctx context.Context, user uuid.UUID, p core.Progress) (bool, error)
	Continue(ctx context.Context, user uuid.UUID, limit int) ([]core.Progress, error)
	History(ctx context.Context, user uuid.UUID, before *Cursor, limit int) ([]core.View, error)
	Watchlist(ctx context.Context, user uuid.UUID) ([]WatchItem, error)
	AddWatch(ctx context.Context, user, title uuid.UUID) error
	RemoveWatch(ctx context.Context, user, title uuid.UUID) error
	InWatchlist(ctx context.Context, user, title uuid.UUID) (bool, error)
}

type WatchItem struct {
	TitleID uuid.UUID `json:"title_id"`
	AddedAt time.Time `json:"added_at"`
}

type Cursor struct {
	At time.Time
	ID uuid.UUID
}

type Service struct {
	Store     Store
	Access    Access
	StreamKey []byte
	StreamTTL time.Duration
	Clock     func() time.Time
	// HasMedia — есть ли файл в хранилище; если нет, играет демо-ролик.
	HasMedia func(name string) bool
}

// DemoMedia — ролик-заглушка для тайтлов, у которых видео ещё не загружено.
const DemoMedia = "sample.mp4"

func (s *Service) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}

func viewer(ctx context.Context) (principal.Principal, uuid.UUID, error) {
	// Администратор тоже может смотреть: проверить карточку, серии, поток.
	p, err := principal.Need(ctx, principal.Viewer, principal.Admin)
	if err != nil {
		return p, uuid.Nil, err
	}
	return p, uuid.MustParse(p.UserID), nil
}

type Start struct {
	StreamURL   string          `json:"stream_url"`
	ResumeAtSec int             `json:"resume_at_sec"`
	DurationSec int             `json:"duration_sec"`
	Via         string          `json:"via"`
	Episode     *events.Episode `json:"episode,omitempty"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Demo        bool            `json:"demo"` // видео тайтла нет — показываем демо-ролик
}

// Play — проверки перед стартом: тайтл на витрине → возраст → право
// (подписка/покупка, решает billing по gRPC). Затем запись в историю и
// короткоживущая подписанная ссылка на поток.
func (s *Service) Play(ctx context.Context, titleID uuid.UUID, episodeID *uuid.UUID) (Start, error) {
	p, user, err := viewer(ctx)
	if err != nil {
		return Start{}, err
	}
	t, ok, err := s.Store.Title(ctx, titleID)
	if err != nil {
		return Start{}, err
	}
	if !ok || t.Status != "published" {
		return Start{}, problem.Conflict("not_available", "Тайтл сейчас недоступен для просмотра")
	}
	via := "preview"
	if p.Role != principal.Admin {
		// Зритель: возраст, затем деньги. Администратор смотрит без оплаты —
		// это предпросмотр каталога, а не покупка.
		if err := core.AgeGate(t.AgeRating, p.Age(s.now())); err != nil {
			return Start{}, err
		}
		allowed, v, denial, err := s.Access.Check(ctx, user, titleID)
		if err != nil {
			return Start{}, problem.Unavailable("access_unavailable", "Не удалось проверить подписку, попробуйте через минуту").With(err)
		}
		if !allowed {
			if denial == "not_available" {
				return Start{}, problem.Conflict("not_available", "Тайтл сейчас недоступен для просмотра")
			}
			return Start{}, problem.New(402, "not_entitled", "Оформите подписку или купите фильм, чтобы смотреть")
		}
		via = v
	}

	st := Start{Via: via, DurationSec: t.DurationSec}
	media := t.Media
	prog, err := s.Store.Progress(ctx, user, titleID)
	if err != nil {
		return Start{}, err
	}
	if t.Kind == "series" {
		var ep events.Episode
		var found bool
		switch {
		case episodeID != nil:
			ep, found = t.Episode(*episodeID)
		case prog != nil && prog.EpisodeID != nil:
			ep, found = t.Episode(*prog.EpisodeID)
		}
		if !found {
			if episodeID != nil {
				return Start{}, problem.NotFound("Серия")
			}
			ep, found = t.FirstEpisode()
		}
		if !found {
			return Start{}, problem.Conflict("no_episodes", "У сериала пока нет серий")
		}
		st.Episode, st.DurationSec = &ep, ep.DurationSec
		media = ep.Media
		epID := uuid.MustParse(ep.ID)
		episodeID = &epID
	}
	if prog != nil && sameEpisode(prog.EpisodeID, episodeID) {
		st.ResumeAtSec = prog.ResumeAt()
	}
	view := core.View{ID: uuid.New(), TitleID: titleID, EpisodeID: episodeID, Via: via, StartedAt: s.now()}
	if err := s.Store.RecordView(ctx, user, view); err != nil {
		return Start{}, err
	}
	exp := s.now().Add(s.ttl())
	st.ExpiresAt = exp
	if media == "" || s.HasMedia == nil || !s.HasMedia(media) {
		media, st.Demo = DemoMedia, true
	}
	st.StreamURL = "/api/playback/stream/" + s.streamToken(user, titleID, media, exp)
	return st, nil
}

func sameEpisode(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Service) ttl() time.Duration {
	if s.StreamTTL > 0 {
		return s.StreamTTL
	}
	return 4 * time.Hour
}

// Ссылка на поток — подписанный токен «кто, что, до когда». Плееру (<video>)
// не нужны заголовки: токен в URL сам по себе пропуск, и проверить его
// может любой узел раздачи без похода в billing.
func (s *Service) streamToken(user, title uuid.UUID, media string, exp time.Time) string {
	payload := fmt.Sprintf("%s|%s|%d|%s", user, title, exp.Unix(), media)
	m := hmac.New(sha256.New, s.StreamKey)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

// VerifyStream проверяет токен потока и возвращает тайтл и имя файла.
func (s *Service) VerifyStream(tok string) (uuid.UUID, string, error) {
	bad := problem.New(403, "bad_stream_token", "Ссылка на видео недействительна — откройте фильм заново")
	parts := strings.SplitN(tok, ".", 2)
	if len(parts) != 2 {
		return uuid.Nil, "", bad
	}
	raw, err1 := base64.RawURLEncoding.DecodeString(parts[0])
	sig, err2 := base64.RawURLEncoding.DecodeString(parts[1])
	if err1 != nil || err2 != nil {
		return uuid.Nil, "", bad
	}
	m := hmac.New(sha256.New, s.StreamKey)
	m.Write(raw)
	if !hmac.Equal(sig, m.Sum(nil)) {
		return uuid.Nil, "", bad
	}
	f := strings.Split(string(raw), "|")
	if len(f) != 4 {
		return uuid.Nil, "", bad
	}
	exp, err := strconv.ParseInt(f[2], 10, 64)
	if err != nil || s.now().Unix() > exp {
		return uuid.Nil, "", problem.New(403, "stream_expired", "Ссылка на видео устарела — откройте фильм заново")
	}
	id, err := uuid.Parse(f[1])
	if err != nil {
		return uuid.Nil, "", bad
	}
	return id, f[3], nil
}

func (s *Service) SaveProgress(ctx context.Context, title uuid.UUID, ep *uuid.UUID, pos, dur int, clientMs int64) (core.Progress, bool, error) {
	_, user, err := viewer(ctx)
	if err != nil {
		return core.Progress{}, false, err
	}
	pr, err := core.NewProgress(title, ep, pos, dur, clientMs, s.now())
	if err != nil {
		return pr, false, err
	}
	applied, err := s.Store.SaveProgress(ctx, user, pr)
	return pr, applied, err
}

func (s *Service) ProgressFor(ctx context.Context, title uuid.UUID) (*core.Progress, bool, error) {
	_, user, err := viewer(ctx)
	if err != nil {
		return nil, false, err
	}
	p, err := s.Store.Progress(ctx, user, title)
	if err != nil {
		return nil, false, err
	}
	in, err := s.Store.InWatchlist(ctx, user, title)
	return p, in, err
}

func (s *Service) Continue(ctx context.Context) ([]core.Progress, map[uuid.UUID]core.Title, error) {
	_, user, err := viewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	list, err := s.Store.Continue(ctx, user, 20)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]uuid.UUID, 0, len(list))
	for _, p := range list {
		ids = append(ids, p.TitleID)
	}
	ts, err := s.Store.Titles(ctx, ids)
	return list, ts, err
}

func (s *Service) History(ctx context.Context, before *Cursor) ([]core.View, map[uuid.UUID]core.Title, error) {
	_, user, err := viewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	list, err := s.Store.History(ctx, user, before, 30)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]uuid.UUID, 0, len(list))
	for _, v := range list {
		ids = append(ids, v.TitleID)
	}
	ts, err := s.Store.Titles(ctx, ids)
	return list, ts, err
}

func (s *Service) Watchlist(ctx context.Context) ([]WatchItem, map[uuid.UUID]core.Title, error) {
	_, user, err := viewer(ctx)
	if err != nil {
		return nil, nil, err
	}
	list, err := s.Store.Watchlist(ctx, user)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]uuid.UUID, 0, len(list))
	for _, w := range list {
		ids = append(ids, w.TitleID)
	}
	ts, err := s.Store.Titles(ctx, ids)
	return list, ts, err
}

// AddWatch — в «Смотреть позже» можно добавить только то, что есть на
// витрине; идемпотентно.
func (s *Service) AddWatch(ctx context.Context, title uuid.UUID) error {
	_, user, err := viewer(ctx)
	if err != nil {
		return err
	}
	t, ok, err := s.Store.Title(ctx, title)
	if err != nil {
		return err
	}
	if !ok || t.Status != "published" {
		return problem.NotFound("Тайтл")
	}
	return s.Store.AddWatch(ctx, user, title)
}

func (s *Service) RemoveWatch(ctx context.Context, title uuid.UUID) error {
	_, user, err := viewer(ctx)
	if err != nil {
		return err
	}
	return s.Store.RemoveWatch(ctx, user, title)
}

// Forgetter — кэш решений о доступе, который можно сбросить по событию.
type Forgetter interface {
	Forget(user, title uuid.UUID)
	ForgetUser(user uuid.UUID)
}

// HandleOrders — потребитель billing.orders.v1: проекция заказа и, при
// возврате (refunding — право в billing уже забрано, refunded — деньги
// вернулись), сброс кэша решений о доступе — после фиксации (а не до:
// если фиксация упадёт, событие придёт снова, а лишний сброс безвреден).
// Кэшируются только разрешения, поэтому новая покупка сброса не требует —
// а сброс на каждую покупку стёр бы и запас на случай аварии billing (5.3).
func (s *Service) HandleOrders(ctx context.Context, env events.Envelope) error {
	o, err := bus.Decode[events.OrderSnapshot](env)
	if err != nil {
		return err
	}
	// Корректный JSON с негодными значениями — тоже яд: повтор не поможет,
	// событие уходит в DLQ, а не вешает партицию.
	if _, err := uuid.Parse(o.OrderID); err != nil {
		return fmt.Errorf("%w: order_id: %v", bus.Permanent, err)
	}
	user, err := uuid.Parse(o.UserID)
	if err != nil {
		return fmt.Errorf("%w: user_id: %v", bus.Permanent, err)
	}
	if o.TitleID != "" {
		if _, err := uuid.Parse(o.TitleID); err != nil {
			return fmt.Errorf("%w: title_id: %v", bus.Permanent, err)
		}
	}
	applied, err := s.Store.ApplyOrder(ctx, env, o)
	if err != nil || !applied {
		return err
	}
	f, ok := s.Access.(Forgetter)
	if !ok || (o.Status != "refunding" && o.Status != "refunded") {
		return nil
	}
	if t, err := uuid.Parse(o.TitleID); err == nil {
		f.Forget(user, t)
	} else {
		f.ForgetUser(user) // подписка открывает или закрывает всё
	}
	return nil
}

func (s *Service) HandleCatalog(ctx context.Context, env events.Envelope) error {
	snap, err := bus.Decode[events.TitleSnapshot](env)
	if err != nil {
		return err
	}
	return s.Store.ApplyTitle(ctx, env, snap)
}
