package services

import (
	"log/slog"
	"sync"

	"windshift/internal/database"
)

// historyRequest represents an async request to record item creation history
type historyRequest struct {
	db     database.Database
	itemID int
	userID int
}

// HistoryService handles asynchronous history recording to avoid blocking the hot path.
// It uses a buffered channel and background goroutine, following the same pattern as NotificationService.
type HistoryService struct {
	historyChan chan historyRequest
	stopChan    chan struct{}
	wg          sync.WaitGroup
}

const historyBufferSize = 1000

var (
	globalHistoryService *HistoryService
	historyOnce          sync.Once
)

// GetHistoryService returns the singleton HistoryService, creating it on first call.
func GetHistoryService(db database.Database) *HistoryService {
	historyOnce.Do(func() {
		globalHistoryService = newHistoryService()
	})
	return globalHistoryService
}

func newHistoryService() *HistoryService {
	hs := &HistoryService{
		historyChan: make(chan historyRequest, historyBufferSize),
		stopChan:    make(chan struct{}),
	}
	hs.wg.Add(1)
	go hs.processor()
	return hs
}

// RecordItemCreationHistoryAsync queues item creation history to be written in the background.
func (hs *HistoryService) RecordItemCreationHistoryAsync(db database.Database, itemID, userID int) {
	select {
	case hs.historyChan <- historyRequest{db: db, itemID: itemID, userID: userID}:
		// Queued successfully
	default:
		// Channel full — drop and log
		slog.Warn("history channel full, dropping creation history",
			slog.Int("item_id", itemID))
	}
}

// processor drains the channel and writes history entries in the background.
func (hs *HistoryService) processor() {
	defer hs.wg.Done()

	for {
		select {
		case req := <-hs.historyChan:
			updateService := NewItemUpdateService(req.db)
			if err := updateService.recordItemCreationHistory(req.db, req.itemID, req.userID); err != nil {
				slog.Warn("async: failed to record item creation history",
					slog.Int("item_id", req.itemID), slog.Any("error", err))
			}
		case <-hs.stopChan:
			// Drain remaining
			for len(hs.historyChan) > 0 {
				req := <-hs.historyChan
				updateService := NewItemUpdateService(req.db)
				if err := updateService.recordItemCreationHistory(req.db, req.itemID, req.userID); err != nil {
					slog.Warn("async shutdown: failed to record item creation history",
						slog.Int("item_id", req.itemID), slog.Any("error", err))
				}
			}
			return
		}
	}
}

// Close gracefully shuts down the history service.
func (hs *HistoryService) Close() {
	close(hs.stopChan)
	hs.wg.Wait()
}
