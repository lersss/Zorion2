// internal/generator/status.go
package generator

import (
	"context"
	"sync"
)

type JobType string

const (
	JobGenerateUniverse JobType = "generate_universe"
	JobGeneratePlanets  JobType = "generate_planets"
	JobGenerateFactions JobType = "generate_factions"
)

type JobStatus struct {
	mu         sync.RWMutex
	Total      int
	Processed  int
	Status     string
	Error      string
	CancelFunc context.CancelFunc
}

type StatusManager struct {
	mu   sync.RWMutex
	jobs map[JobType]*JobStatus
}

func NewStatusManager() *StatusManager {
	return &StatusManager{
		jobs: make(map[JobType]*JobStatus),
	}
}

func (sm *StatusManager) Get(job JobType) *JobStatus {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.jobs[job]
}

// TryStart — атомарно: если задача уже running, вернёт false.
// Если свободна — создаёт JobStatus и возвращает true.
// Это убирает race между проверкой статуса и стартом.
func (sm *StatusManager) TryStart(job JobType, total int, cancel context.CancelFunc) bool {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if existing, ok := sm.jobs[job]; ok {
		existing.mu.RLock()
		isRunning := existing.Status == "running"
		existing.mu.RUnlock()
		if isRunning {
			return false
		}
	}

	sm.jobs[job] = &JobStatus{
		Total:      total,
		Processed:  0,
		Status:     "running",
		Error:      "",
		CancelFunc: cancel,
	}
	return true
}

// Start — legacy-метод, оставлен на случай ручного использования.
// В новых вызовах используй TryStart.
func (sm *StatusManager) Start(job JobType, total int, cancel context.CancelFunc) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.jobs[job] = &JobStatus{
		Total:      total,
		Processed:  0,
		Status:     "running",
		CancelFunc: cancel,
	}
}

// IsRunning — атомарная проверка «идёт ли задача».
func (sm *StatusManager) IsRunning(job JobType) bool {
	s := sm.Get(job)
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Status == "running"
}

func (sm *StatusManager) Progress(job JobType, processed int) {
	s := sm.Get(job)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Processed = processed
}

func (sm *StatusManager) Done(job JobType) {
	s := sm.Get(job)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "done"
	s.CancelFunc = nil
}

func (sm *StatusManager) Fail(job JobType, err string) {
	s := sm.Get(job)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Status = "error"
	s.Error = err
	s.CancelFunc = nil
}

func (sm *StatusManager) Cancel(job JobType) {
	s := sm.Get(job)
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Status != "running" {
		return
	}
	if s.CancelFunc != nil {
		s.CancelFunc()
	}
	s.Status = "canceled"
	s.CancelFunc = nil
}

func (sm *StatusManager) GetStatus(job JobType) (total, processed int, status, err string) {
	s := sm.Get(job)
	if s == nil {
		return 0, 0, "idle", ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Total, s.Processed, s.Status, s.Error
}