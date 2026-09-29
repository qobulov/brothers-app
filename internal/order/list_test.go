package order

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/qobulov/brothers-app/pkg/apperror"
)

func TestList_ScopesEmployeesFiltersAndPages(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	first := f.create(f.giver)
	input := f.createInput()
	input.GiverUserID = f.bystander
	second, err := f.service.Create(ctx, f.manager, f.groupID, input)
	if err != nil {
		t.Fatalf("create second order: %v", err)
	}

	visible := map[uuid.UUID]int{f.giver: 1, f.bystander: 1, f.receiver: 2, f.manager: 2, f.owner: 2, f.investor: 2}
	for actor, want := range visible {
		items, err := f.service.List(ctx, actor, f.groupID, ListInput{})
		if err != nil {
			t.Fatalf("list as %s: %v", actor, err)
		}
		if len(items) != want {
			t.Fatalf("list as %s = %d orders, want %d", actor, len(items), want)
		}
	}

	all, err := f.service.List(ctx, f.manager, f.groupID, ListInput{})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if all[0].ID != second.ID || all[1].ID != first.ID {
		t.Fatal("orders must be newest first")
	}
	if all[0].Giver.LocationName != "" || all[1].Giver.LocationName != "Tashkent" || all[1].Receiver.Name != "aziz" {
		t.Fatalf("list items = %#v", all)
	}
	if all[1].State != StateWaitingForConfirmation {
		t.Fatalf("manager state = %s, want waiting_for_confirmation", all[1].State)
	}
	mine, _ := f.service.List(ctx, f.giver, f.groupID, ListInput{})
	if mine[0].State != StateWaitingForYou {
		t.Fatalf("giver state = %s, want waiting_for_you", mine[0].State)
	}

	page, err := f.service.List(ctx, f.manager, f.groupID, ListInput{Limit: 1, Offset: 1})
	if err != nil || len(page) != 1 || page[0].ID != first.ID {
		t.Fatalf("second page = %#v, %v; want the first order", page, err)
	}
	completed, err := f.service.List(ctx, f.manager, f.groupID, ListInput{Status: "COMPLETED"})
	if err != nil || len(completed) != 0 {
		t.Fatalf("completed filter = %#v, %v; want none", completed, err)
	}
	for _, bad := range []ListInput{{Status: "bogus"}, {Limit: maxPageSize + 1}, {Limit: -1}, {Offset: -1}, {Offset: maxOffset + 1}} {
		if _, err := f.service.List(ctx, f.manager, f.groupID, bad); !errors.Is(err, apperror.ErrInvalidData) {
			t.Fatalf("list %#v error = %v, want invalid data", bad, err)
		}
	}
	if _, err := f.service.List(ctx, f.stranger, f.groupID, ListInput{}); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("stranger list error = %v, want not found", err)
	}
}

func TestList_DeletedGroupHidesOrders(t *testing.T) {
	f := newFixture(t)
	created := f.create(f.manager)
	f.exec(`UPDATE groups SET is_active = false, deleted_at = now() WHERE id = $1`, f.groupID)

	if _, err := f.service.List(context.Background(), f.manager, f.groupID, ListInput{}); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("list error = %v, want not found", err)
	}
	if _, err := f.service.Get(context.Background(), f.manager, f.groupID, created.ID); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("get error = %v, want not found", err)
	}
}
