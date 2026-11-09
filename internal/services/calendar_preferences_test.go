package services

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
	"windshift/internal/database"
	"windshift/internal/models"
	"windshift/internal/repository"
)

func TestCalendarPreferenceDefaults(t *testing.T) {
	for _, tc := range []struct{ input, want string }{{"", "auto"}, {"unknown", "auto"}, {"gregory", "gregory"}, {"persian", "persian"}} {
		if got := normalizeCalendar(tc.input); got != tc.want {
			t.Errorf("normalizeCalendar(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestCalendarPreferenceConcurrentFirstWrites(t *testing.T) {
	db, err := database.NewSQLiteDBWithPoolSizes(filepath.Join(t.TempDir(), "prefs.db"), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.ExecWrite("CREATE TABLE user_preferences (user_id INTEGER UNIQUE, preferences TEXT NOT NULL, created_at DATETIME, updated_at DATETIME)")
	if err != nil {
		t.Fatal(err)
	}
	service := &UserPreferencesService{prefsRepo: repository.NewUserPreferencesRepository(db)}
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	errors := make(chan error, 2)
	var calls atomic.Int32
	update := func(fn func(*models.UserPreferencesData)) {
		errors <- service.updateData(7, func(p *models.UserPreferencesData) error {
			if calls.Add(1) <= 2 {
				ready <- struct{}{}
				<-release
			}
			fn(p)
			return nil
		})
	}
	go update(func(p *models.UserPreferencesData) { p.Calendar = "persian" })
	go update(func(p *models.UserPreferencesData) { p.ColorMode = "dark" })
	for range 2 {
		select {
		case <-ready:
		case err := <-errors:
			close(release)
			t.Fatalf("preparing concurrent write: %v", err)
		case <-time.After(10 * time.Second):
			close(release)
			t.Fatal("timed out preparing concurrent writes")
		}
	}
	close(release)
	for range 2 {
		if err := <-errors; err != nil {
			t.Fatal(err)
		}
	}
	prefs, err := service.loadData(7)
	if err != nil {
		t.Fatal(err)
	}
	if prefs.Calendar != "persian" || prefs.ColorMode != "dark" {
		t.Fatalf("concurrent updates lost: %+v", prefs)
	}
}

func TestCalendarPreferenceConflictAfterRetries(t *testing.T) {
	db, err := database.NewSQLiteDBWithPoolSizes(filepath.Join(t.TempDir(), "prefs.db"), 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.ExecWrite("CREATE TABLE user_preferences (user_id INTEGER UNIQUE, preferences TEXT NOT NULL, created_at DATETIME, updated_at DATETIME)")
	if err != nil {
		t.Fatal(err)
	}
	repo := repository.NewUserPreferencesRepository(db)
	if ok, err := repo.CompareAndSwapJSON(7, nil, "{}", time.Now()); !ok || err != nil {
		t.Fatalf("seed: %v %v", ok, err)
	}
	service := &UserPreferencesService{prefsRepo: repo}
	calls := 0
	err = service.updateData(7, func(p *models.UserPreferencesData) error {
		calls++
		old, err := repo.GetJSON(7)
		if err != nil {
			return err
		}
		_, err = repo.CompareAndSwapJSON(7, &old, fmt.Sprintf("{\"color_mode\":\"writer%d\"}", calls), time.Now())
		p.Calendar = "persian"
		return err
	})
	if !errors.Is(err, repository.ErrConcurrentUpdate) || calls != 3 {
		t.Fatalf("got %v after %d calls", err, calls)
	}
}
