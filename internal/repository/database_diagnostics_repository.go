package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"windshift/internal/database"
)

// DatabasePoolStats is a point-in-time view of one database/sql pool.
// Wait and close counters are cumulative since the pool was created.
type DatabasePoolStats struct {
	Name               string
	Role               string
	Driver             string
	MaxOpenConnections int
	OpenConnections    int
	InUse              int
	Idle               int
	WaitCount          int64
	WaitDurationMillis int64
	MaxIdleClosed      int64
	MaxIdleTimeClosed  int64
	MaxLifetimeClosed  int64
}

// Key uniquely identifies a pool within the process for alert state.
func (s DatabasePoolStats) Key() string {
	if s.Role == "" {
		return s.Name
	}
	return s.Name + "/" + s.Role
}

// RequestQueryOutcomeStats is the repository-layer projection of request-owned
// query terminal outcomes.
type RequestQueryOutcomeStats struct {
	Canceled  uint64 `json:"canceled"`
	Deadlines uint64 `json:"deadlines"`
	Errors    uint64 `json:"errors"`
	Successes uint64 `json:"successes"`
}

// DatabaseCapacityBudget records the replica-aware PostgreSQL connection
// calculation made at startup. AuxiliaryConnectionsPerReplica includes pools
// such as the optional SSH authentication pool.
type DatabaseCapacityBudget struct {
	ServerMaxConnections           int
	MainConnectionsPerReplica      int
	AuxiliaryConnectionsPerReplica int
	ConnectionsPerReplica          int
	ReplicaCount                   int
	HeadroomConnections            int
	RequiredConnections            int
	RemainingConnections           int
	UtilizationPercent             float64
	Safe                           bool
}

// DatabaseDiagnosticsRepository owns the process-local registry of SQL pools.
// Pools can be registered after HTTP startup (the SSH pool is initialized by
// main) while diagnostics and the monitor safely read snapshots concurrently.
type DatabaseDiagnosticsRepository struct {
	mu       sync.RWMutex
	pools    map[string]database.Database
	capacity *DatabaseCapacityBudget
}

func NewDatabaseDiagnosticsRepository(db database.Database) *DatabaseDiagnosticsRepository {
	repo := &DatabaseDiagnosticsRepository{pools: make(map[string]database.Database)}
	if db != nil {
		repo.pools["main"] = db
	}
	return repo
}

// RegisterPool adds or replaces a named process-local SQL pool.
func (r *DatabaseDiagnosticsRepository) RegisterPool(name string, db database.Database) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("database pool name is required")
	}
	if db == nil {
		return fmt.Errorf("database pool %q is nil", name)
	}
	r.mu.Lock()
	r.pools[name] = db
	r.mu.Unlock()
	return nil
}

// PoolStats returns stable name-sorted snapshots for every registered pool.
func (r *DatabaseDiagnosticsRepository) PoolStats() []DatabasePoolStats {
	r.mu.RLock()
	names := make([]string, 0, len(r.pools))
	pools := make(map[string]database.Database, len(r.pools))
	for name, db := range r.pools {
		names = append(names, name)
		pools[name] = db
	}
	r.mu.RUnlock()
	sort.Strings(names)

	snapshots := make([]DatabasePoolStats, 0, len(names))
	for _, name := range names {
		db := pools[name]
		for _, pool := range db.NamedPools() {
			if pool.DB == nil {
				continue
			}
			stats := pool.DB.Stats()
			snapshots = append(snapshots, DatabasePoolStats{
				Name:               name,
				Role:               pool.Name,
				Driver:             db.GetDriverName(),
				MaxOpenConnections: stats.MaxOpenConnections,
				OpenConnections:    stats.OpenConnections,
				InUse:              stats.InUse,
				Idle:               stats.Idle,
				WaitCount:          stats.WaitCount,
				WaitDurationMillis: stats.WaitDuration.Milliseconds(),
				MaxIdleClosed:      stats.MaxIdleClosed,
				MaxIdleTimeClosed:  stats.MaxIdleTimeClosed,
				MaxLifetimeClosed:  stats.MaxLifetimeClosed,
			})
		}
	}
	return snapshots
}

// RequestQueryStats returns process-local request query outcome counters.
func (r *DatabaseDiagnosticsRepository) RequestQueryStats() RequestQueryOutcomeStats {
	stats := database.RequestQueryStats()
	return RequestQueryOutcomeStats{
		Canceled:  stats.Canceled,
		Deadlines: stats.Deadlines,
		Errors:    stats.Errors,
		Successes: stats.Successes,
	}
}

// RequestTransactionStats returns process-local committed-transaction counters.
func (r *DatabaseDiagnosticsRepository) RequestTransactionStats() database.RequestTransactionStats {
	return database.RequestTransactionStatsSnapshot()
}

// LoadPostgresCapacityBudget reads the server capacity and calculates the
// declared deployment's aggregate connection reservation.
func (r *DatabaseDiagnosticsRepository) LoadPostgresCapacityBudget(
	ctx context.Context,
	main database.Database,
	replicas, headroom, auxiliaryPerReplica int,
) (DatabaseCapacityBudget, error) {
	if main == nil || main.GetDriverName() != "postgres" {
		return DatabaseCapacityBudget{}, fmt.Errorf("PostgreSQL database is required for capacity budgeting")
	}
	var serverMax int
	if err := main.QueryRowContext(ctx, `SELECT current_setting('max_connections')::int`).Scan(&serverMax); err != nil {
		return DatabaseCapacityBudget{}, fmt.Errorf("read PostgreSQL max_connections: %w", err)
	}
	budget, err := CalculateDatabaseCapacityBudget(
		serverMax,
		main.GetDB().Stats().MaxOpenConnections,
		auxiliaryPerReplica,
		replicas,
		headroom,
	)
	if err != nil {
		return DatabaseCapacityBudget{}, err
	}
	r.SetCapacityBudget(budget)
	return budget, nil
}

// CalculateDatabaseCapacityBudget evaluates the documented deployment formula.
func CalculateDatabaseCapacityBudget(
	serverMax, mainPerReplica, auxiliaryPerReplica, replicas, headroom int,
) (DatabaseCapacityBudget, error) {
	if serverMax <= 0 {
		return DatabaseCapacityBudget{}, fmt.Errorf("PostgreSQL max_connections must be positive")
	}
	if mainPerReplica <= 0 {
		return DatabaseCapacityBudget{}, fmt.Errorf("main pool size must be positive")
	}
	if auxiliaryPerReplica < 0 || headroom < 0 {
		return DatabaseCapacityBudget{}, fmt.Errorf("auxiliary connections and headroom cannot be negative")
	}
	if replicas <= 0 {
		return DatabaseCapacityBudget{}, fmt.Errorf("replica count must be positive")
	}
	perReplica := mainPerReplica + auxiliaryPerReplica
	required := perReplica*replicas + headroom
	remaining := serverMax - required
	return DatabaseCapacityBudget{
		ServerMaxConnections:           serverMax,
		MainConnectionsPerReplica:      mainPerReplica,
		AuxiliaryConnectionsPerReplica: auxiliaryPerReplica,
		ConnectionsPerReplica:          perReplica,
		ReplicaCount:                   replicas,
		HeadroomConnections:            headroom,
		RequiredConnections:            required,
		RemainingConnections:           remaining,
		UtilizationPercent:             float64(required) / float64(serverMax) * 100,
		Safe:                           required <= serverMax,
	}, nil
}

func (r *DatabaseDiagnosticsRepository) SetCapacityBudget(budget DatabaseCapacityBudget) {
	r.mu.Lock()
	copyOfBudget := budget
	r.capacity = &copyOfBudget
	r.mu.Unlock()
}

func (r *DatabaseDiagnosticsRepository) CapacityBudget() *DatabaseCapacityBudget {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.capacity == nil {
		return nil
	}
	copyOfBudget := *r.capacity
	return &copyOfBudget
}
