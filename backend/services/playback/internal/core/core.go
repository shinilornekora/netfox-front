// Package core — правила просмотра.
package core

import (
	"time"

	"github.com/google/uuid"

	"plenka/contracts/events"
	"plenka/kit/problem"
)

type Title struct {
	ID          uuid.UUID
	Name        string
	Kind        string
	Year        int
	AgeRating   int
	Status      string
	DurationSec int
	PosterHue   int
	PosterRev   int64
	Media       string
	Episodes    []events.Episode
}

func (t Title) Episode(id uuid.UUID) (events.Episode, bool) {
	for _, e := range t.Episodes {
		if e.ID == id.String() {
			return e, true
		}
	}
	return events.Episode{}, false
}

// FirstEpisode — по умолчанию сериал начинается с 1×1.
func (t Title) FirstEpisode() (events.Episode, bool) {
	var best events.Episode
	found := false
	for _, e := range t.Episodes {
		if !found || e.Season < best.Season || (e.Season == best.Season && e.Number < best.Number) {
			best, found = e, true
		}
	}
	return best, found
}

// AgeGate — возрастная маркировка. Если дата рождения не указана, 18+
// закрыт: «не знаем возраст» нельзя считать «взрослый».
func AgeGate(rating, age int) error {
	if rating == 0 {
		return nil
	}
	if age == 0 && rating >= 18 {
		return problem.Forbidden("Для фильмов 18+ укажите дату рождения в профиле")
	}
	if age > 0 && age < rating {
		return problem.Forbidden("Этот тайтл недоступен по возрасту")
	}
	return nil
}

// FinishedShare — с какой доли длительности считаем просмотренным
// (финальные титры обычно не досматривают).
const FinishedShare = 0.93

type Progress struct {
	TitleID     uuid.UUID  `json:"title_id"`
	EpisodeID   *uuid.UUID `json:"episode_id,omitempty"`
	PositionSec int        `json:"position_sec"`
	DurationSec int        `json:"duration_sec"`
	Finished    bool       `json:"finished"`
	ClientMs    int64      `json:"-"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// NewProgress проверяет «пинг» прогресса от плеера.
// MaxClockSkew — насколько часы устройства могут опережать сервер.
const MaxClockSkew = 5 * time.Minute

func NewProgress(title uuid.UUID, ep *uuid.UUID, pos, dur int, clientMs int64, now time.Time) (Progress, error) {
	f := problem.Fields{}
	if dur <= 0 || dur > 6*3600 {
		f.Set("duration_sec", "Некорректная длительность")
	}
	if pos < 0 || pos > dur+5 {
		f.Set("position_sec", "Позиция вне длительности")
	}
	// Время устройства решает, какой пинг новее. Часы, убежавшие в будущее,
	// «заморозили» бы прогресс навсегда — такие пинги не принимаем.
	if clientMs <= 0 || clientMs > now.Add(MaxClockSkew).UnixMilli() {
		f.Set("client_ms", "Часы устройства расходятся с сервером — проверьте дату и время")
	}
	if err := f.Err(); err != nil {
		return Progress{}, err
	}
	if pos > dur {
		pos = dur
	}
	return Progress{TitleID: title, EpisodeID: ep, PositionSec: pos, DurationSec: dur,
		Finished: float64(pos) >= float64(dur)*FinishedShare, ClientMs: clientMs, UpdatedAt: now}, nil
}

// ResumeAt — с какого места продолжить: досмотренное начинаем сначала,
// а последние секунды перед точкой остановки показываем повторно, чтобы
// зритель вспомнил, на чём остановился.
func (p Progress) ResumeAt() int {
	if p.Finished {
		return 0
	}
	if p.PositionSec > 5 {
		return p.PositionSec - 5
	}
	return 0
}

type View struct {
	ID        uuid.UUID  `json:"id"`
	TitleID   uuid.UUID  `json:"title_id"`
	EpisodeID *uuid.UUID `json:"episode_id,omitempty"`
	Via       string     `json:"via"`
	StartedAt time.Time  `json:"started_at"`
}
