package adk

import "testing"

func TestValidateSQLiteSessionServiceAcceptsCurrentSchema(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	service, err := NewSQLiteSessionService(dir + "/adk-session.db")
	if err != nil {
		t.Fatalf("NewSQLiteSessionService: %v", err)
	}
	t.Cleanup(func() {
		jftradeErr1 := service.Close()
		jftradeCheckTestError(t, jftradeErr1)
	})

	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("first ValidateSQLiteSessionService: %v", err)
	}
	if err := ValidateSQLiteSessionService(service); err != nil {
		t.Fatalf("second ValidateSQLiteSessionService: %v", err)
	}

	ready, err := sqliteSessionSchemaReady(service.db)
	if err != nil {
		t.Fatalf("sqliteSessionSchemaReady: %v", err)
	}
	if !ready {
		t.Fatal("expected sqlite session schema to be ready")
	}
}
