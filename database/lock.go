package database

import (
	"fmt"
	"os"
	"syscall"
)

// migrationLockSuffix names the advisory lock file that serializes schema
// migrations for a given db-path across processes.
const migrationLockSuffix = ".migrate.lock"

// AcquireMigrationLock blocks until it holds an exclusive, cross-process
// advisory lock (flock) guarding schema migrations for dbPath, and returns a
// function that releases it.
//
// golang-migrate's sqlite driver does not coordinate concurrent migrators
// across processes: two theia processes migrating the same db-path at once race
// on its schema_migrations dirty-state bookkeeping and one aborts with
// "Dirty database version N. Fix and force version." The busy_timeout pragma
// does not help here because the conflict is golang-migrate's own logic, not a
// raw SQLite lock. Serializing migrations behind this lock makes the loser wait
// for the winner to finish and then observe an already-migrated schema
// (migrate.ErrNoChange). See issue #23.
//
// The lock is advisory and released automatically if the holding process exits,
// so a crash mid-migration cannot wedge later starts. flock is available on the
// unix targets theia runs on (it already relies on `tail -f`).
func AcquireMigrationLock(dbPath string) (func() error, error) {
	lockPath := dbPath + migrationLockSuffix

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lockPath derives from the operator-provided db-path flag, not user input
	if err != nil {
		return nil, fmt.Errorf("opening migration lock %q: %w", lockPath, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		closeErr := f.Close()
		if closeErr != nil {
			return nil, fmt.Errorf("acquiring migration lock %q: %w (and closing it: %w)", lockPath, err, closeErr)
		}
		return nil, fmt.Errorf("acquiring migration lock %q: %w", lockPath, err)
	}

	release := func() error {
		// Closing the descriptor releases the flock held on it.
		if err := f.Close(); err != nil {
			return fmt.Errorf("releasing migration lock %q: %w", lockPath, err)
		}
		return nil
	}

	return release, nil
}
