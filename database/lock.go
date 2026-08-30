package database

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"
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

	// os.File.Fd() returns a kernel file descriptor that always fits in an int;
	// the uintptr->int conversion is what syscall.Flock requires.
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

// AcquireMigrationLockTimeout is AcquireMigrationLock's bounded-wait variant:
// it polls a non-blocking flock instead of blocking indefinitely on
// LOCK_EX, giving up once timeout elapses. ok is false (with a nil error)
// when the lock stayed contended for the whole timeout — the caller's own
// step degrades rather than the process hanging. Use this instead of
// AcquireMigrationLock wherever holding a contended lock forever would
// contradict a caller's own promise that no single step blocks the rest of
// its work (e.g. theia diagnose).
func AcquireMigrationLockTimeout(dbPath string, timeout time.Duration) (release func() error, ok bool, err error) {
	lockPath := dbPath + migrationLockSuffix

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600) //nolint:gosec // lockPath derives from the operator-provided db-path flag, not user input
	if err != nil {
		return nil, false, fmt.Errorf("opening migration lock %q: %w", lockPath, err)
	}

	deadline := time.Now().Add(timeout)
	backoff := 10 * time.Millisecond
	for {
		flockErr := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if flockErr == nil {
			break
		}
		if !errors.Is(flockErr, syscall.EWOULDBLOCK) {
			closeErr := f.Close()
			if closeErr != nil {
				return nil, false, fmt.Errorf("acquiring migration lock %q: %w (and closing it: %w)", lockPath, flockErr, closeErr)
			}
			return nil, false, fmt.Errorf("acquiring migration lock %q: %w", lockPath, flockErr)
		}
		if time.Now().After(deadline) {
			if closeErr := f.Close(); closeErr != nil {
				return nil, false, fmt.Errorf("closing migration lock %q after timeout: %w", lockPath, closeErr)
			}
			return nil, false, nil
		}
		time.Sleep(backoff)
		if backoff < 200*time.Millisecond {
			backoff *= 2
		}
	}

	release = func() error {
		if err := f.Close(); err != nil {
			return fmt.Errorf("releasing migration lock %q: %w", lockPath, err)
		}
		return nil
	}

	return release, true, nil
}
