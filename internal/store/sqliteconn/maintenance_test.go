package sqliteconn

import (
	"sync"
	"testing"
)

func TestCompactSerializesWithConcurrentWritesAndFailsAfterClose(t *testing.T) {
	db, err := Open(t.TempDir() + "/maintenance.db")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE records (id INTEGER PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 9)
	var wg sync.WaitGroup
	wg.Add(9)
	go func() {
		defer wg.Done()
		<-start
		errs <- db.Compact(t.Context())
	}()
	for id := 1; id <= 8; id++ {
		go func() {
			defer wg.Done()
			<-start
			_, writeErr := db.ExecContext(
				t.Context(),
				`INSERT INTO records (id, value) VALUES (?, ?)`,
				id,
				"value",
			)
			errs <- writeErr
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for compactErr := range errs {
		if compactErr != nil {
			t.Fatalf("concurrent maintenance: %v", compactErr)
		}
	}

	var count int
	if err := db.GetContext(t.Context(), &count, `SELECT COUNT(*) FROM records`); err != nil {
		t.Fatal(err)
	}
	if count != 8 {
		t.Fatalf("record count = %d, want 8", count)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if err := db.Compact(t.Context()); err == nil {
		t.Fatal("compact after close succeeded")
	}
}
