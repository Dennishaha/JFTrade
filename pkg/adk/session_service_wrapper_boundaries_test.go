package adk

import (
	"testing"

	adksession "google.golang.org/adk/session"
)

func TestCompactingSessionServiceListDelegatesToBaseService(t *testing.T) {
	ctx := t.Context()
	base := adksession.InMemoryService()
	service := &compactingSessionService{base: base}

	first, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session-a"})
	if err != nil {
		t.Fatalf("Create first: %v", err)
	}
	if _, err := service.Create(ctx, &adksession.CreateRequest{AppName: "app", UserID: "user", SessionID: "session-b"}); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	list, err := service.List(ctx, &adksession.ListRequest{AppName: "app", UserID: "user"})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil || len(list.Sessions) != 2 {
		t.Fatalf("List response = %#v, want two delegated sessions", list)
	}
	if err := service.Delete(ctx, &adksession.DeleteRequest{AppName: "app", UserID: "user", SessionID: first.Session.ID()}); err != nil {
		t.Fatalf("Delete delegated session: %v", err)
	}
	afterDelete, err := service.List(ctx, &adksession.ListRequest{AppName: "app", UserID: "user"})
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if afterDelete == nil || len(afterDelete.Sessions) != 1 {
		t.Fatalf("List after delete = %#v, want one delegated session", afterDelete)
	}
}
