package debt

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/database"
)

type fixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	service *Service
	owner   uuid.UUID
	other   uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool, cleanup := database.SetupTestDB(t)
	t.Cleanup(cleanup)
	f := &fixture{t: t, pool: pool, service: NewService(pool)}
	f.owner = f.user("owner")
	f.other = f.user("other")
	return f
}

func (f *fixture) user(username string) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	err := f.pool.QueryRow(context.Background(), `
		INSERT INTO users (email, username, language) VALUES ($1, $2, 'uz') RETURNING id
	`, username+"@example.com", username).Scan(&id)
	if err != nil {
		f.t.Fatalf("create user: %v", err)
	}
	return id
}

func (f *fixture) create(input CreateInput) Debt {
	f.t.Helper()
	created, err := f.service.Create(context.Background(), f.owner, input)
	if err != nil {
		f.t.Fatalf("create debt: %v", err)
	}
	return created
}

func usd(direction, name string, amount int64) CreateInput {
	return CreateInput{Direction: direction, PersonName: name, Currency: CurrencyUSD, Amount: amount}
}

func TestCreate_ValidatesInput(t *testing.T) {
	f := newFixture(t)
	created := f.create(CreateInput{Direction: " They_Owe_Me ", PersonName: " Akmal ", PersonPhone: "+998 90 777 44 22", Currency: "usd", Amount: 1500})
	if created.Direction != DirectionTheyOweMe || created.PersonName != "Akmal" || created.PersonPhone != "+998907774422" || created.Currency != CurrencyUSD {
		t.Fatalf("created = %#v", created)
	}
	if created.OriginalAmount != 1500 || created.RemainingAmount != 1500 || created.Status != StatusActive || created.CompletedAt != nil {
		t.Fatalf("created amounts/status = %#v", created)
	}

	tests := []struct {
		name  string
		input CreateInput
	}{
		{name: "unknown direction", input: CreateInput{Direction: "lent", PersonName: "A", Currency: "USD", Amount: 1}},
		{name: "empty name", input: CreateInput{Direction: DirectionIOwe, PersonName: "  ", Currency: "USD", Amount: 1}},
		{name: "long name", input: CreateInput{Direction: DirectionIOwe, PersonName: strings.Repeat("a", 101), Currency: "USD", Amount: 1}},
		{name: "bad phone", input: CreateInput{Direction: DirectionIOwe, PersonName: "A", PersonPhone: "abc", Currency: "USD", Amount: 1}},
		{name: "unknown currency", input: CreateInput{Direction: DirectionIOwe, PersonName: "A", Currency: "EUR", Amount: 1}},
		{name: "zero amount", input: CreateInput{Direction: DirectionIOwe, PersonName: "A", Currency: "USD", Amount: 0}},
		{name: "USD above limit", input: CreateInput{Direction: DirectionIOwe, PersonName: "A", Currency: "USD", Amount: 1_000_000_001}},
		{name: "UZS above limit", input: CreateInput{Direction: DirectionIOwe, PersonName: "A", Currency: "UZS", Amount: 1_000_000_000_001}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := f.service.Create(context.Background(), f.owner, tt.input); !errors.Is(err, apperror.ErrInvalidData) {
				t.Fatalf("create error = %v, want invalid data", err)
			}
		})
	}
	// A large UZS amount is fine where the same number in USD is not.
	f.create(CreateInput{Direction: DirectionIOwe, PersonName: "Dilshod", Currency: CurrencyUZS, Amount: 1_000_000_000_000})
}

func TestRepay_PartialThenFinalCompletes(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created := f.create(usd(DirectionTheyOweMe, "Akmal", 1500))

	after, err := f.service.Repay(ctx, f.owner, created.ID, 600)
	if err != nil || after.RemainingAmount != 900 || after.Status != StatusActive {
		t.Fatalf("after partial repayment = %#v, %v", after, err)
	}
	if _, err := f.service.Repay(ctx, f.owner, created.ID, 901); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("overpayment error = %v, want invalid data", err)
	}
	if got := apperror.MessageForLanguage(func() error { _, err := f.service.Repay(ctx, f.owner, created.ID, 901); return err }(), "uz"); got != "To'lov qolgan summadan (900 USD) oshmasligi kerak" {
		t.Fatalf("overpayment message = %q", got)
	}
	if _, err := f.service.Repay(ctx, f.owner, created.ID, 0); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("zero repayment error = %v, want invalid data", err)
	}

	final, err := f.service.Repay(ctx, f.owner, created.ID, 900)
	if err != nil || final.RemainingAmount != 0 || final.Status != StatusCompleted || final.CompletedAt == nil {
		t.Fatalf("after final repayment = %#v, %v", final, err)
	}
	if _, err := f.service.Repay(ctx, f.owner, created.ID, 1); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("repay completed debt error = %v, want conflict", err)
	}

	repayments, err := f.service.ListRepayments(ctx, f.owner, created.ID, Page{})
	if err != nil || len(repayments) != 2 || repayments[0].Amount != 900 || repayments[1].Amount != 600 {
		t.Fatalf("repayments = %#v, %v; want 900 then 600", repayments, err)
	}
}

func TestRepay_ConcurrentRepaymentsCannotOverpay(t *testing.T) {
	f := newFixture(t)
	created := f.create(usd(DirectionTheyOweMe, "Akmal", 1000))

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = f.service.Repay(context.Background(), f.owner, created.ID, 600)
		}()
	}
	wg.Wait()

	succeeded := 0
	for _, err := range errs {
		switch {
		case err == nil:
			succeeded++
		case !errors.Is(err, apperror.ErrInvalidData):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if succeeded != 1 {
		t.Fatalf("successful repayments = %d, want exactly 1", succeeded)
	}
	reloaded, err := f.service.Get(context.Background(), f.owner, created.ID)
	if err != nil || reloaded.RemainingAmount != 400 {
		t.Fatalf("remaining = %#v, %v; want 400", reloaded, err)
	}
}

func TestComplete_KeepsRemainingAmount(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created := f.create(usd(DirectionIOwe, "Akmal", 1500))
	if _, err := f.service.Repay(ctx, f.owner, created.ID, 500); err != nil {
		t.Fatalf("repay: %v", err)
	}

	completed, err := f.service.Complete(ctx, f.owner, created.ID)
	if err != nil || completed.Status != StatusCompleted || completed.RemainingAmount != 1000 || completed.CompletedAt == nil {
		t.Fatalf("completed = %#v, %v; want completed with 1000 left", completed, err)
	}
	if _, err := f.service.Complete(ctx, f.owner, created.ID); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("complete again error = %v, want conflict", err)
	}
}

func TestOwnership_OtherUsersSeeNothing(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created := f.create(usd(DirectionTheyOweMe, "Akmal", 1500))

	checks := map[string]func() error{
		"get":        func() error { _, err := f.service.Get(ctx, f.other, created.ID); return err },
		"repay":      func() error { _, err := f.service.Repay(ctx, f.other, created.ID, 1); return err },
		"repayments": func() error { _, err := f.service.ListRepayments(ctx, f.other, created.ID, Page{}); return err },
		"complete":   func() error { _, err := f.service.Complete(ctx, f.other, created.ID); return err },
		"delete":     func() error { return f.service.Delete(ctx, f.other, created.ID) },
	}
	for name, check := range checks {
		if err := check(); !errors.Is(err, apperror.ErrRecordNotFound) {
			t.Fatalf("%s as another user error = %v, want not found", name, err)
		}
	}
	list, err := f.service.List(ctx, f.other, ListInput{Status: "all"})
	if err != nil || len(list) != 0 {
		t.Fatalf("other user's list = %#v, %v; want empty", list, err)
	}
	summary, err := f.service.Summary(ctx, f.other)
	if err != nil || summary != (Summary{}) {
		t.Fatalf("other user's summary = %#v, %v; want zero", summary, err)
	}
	if _, err := f.service.Get(ctx, f.owner, created.ID); err != nil {
		t.Fatalf("owner still sees the debt: %v", err)
	}
}

func TestDelete_HidesDebtEverywhere(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	created := f.create(usd(DirectionTheyOweMe, "Akmal", 1500))
	if _, err := f.service.Repay(ctx, f.owner, created.ID, 500); err != nil {
		t.Fatalf("repay: %v", err)
	}

	if err := f.service.Delete(ctx, f.owner, created.ID); err != nil {
		t.Fatalf("delete debt with repayments: %v", err)
	}
	if _, err := f.service.Get(ctx, f.owner, created.ID); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("get deleted error = %v, want not found", err)
	}
	if err := f.service.Delete(ctx, f.owner, created.ID); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("delete twice error = %v, want not found", err)
	}
	list, _ := f.service.List(ctx, f.owner, ListInput{Status: "all"})
	summary, _ := f.service.Summary(ctx, f.owner)
	if len(list) != 0 || summary != (Summary{}) {
		t.Fatalf("after delete: list = %d, summary = %#v; want nothing", len(list), summary)
	}
}

func TestSummary_PerCurrencyAndDirection(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	partly := f.create(usd(DirectionTheyOweMe, "Akmal", 1500))
	if _, err := f.service.Repay(ctx, f.owner, partly.ID, 600); err != nil {
		t.Fatalf("repay: %v", err)
	}
	f.create(usd(DirectionTheyOweMe, "Bekzod", 1600))
	f.create(usd(DirectionIOwe, "Akmal", 800))
	f.create(CreateInput{Direction: DirectionTheyOweMe, PersonName: "Sardor", Currency: CurrencyUZS, Amount: 1_975_000})
	f.create(CreateInput{Direction: DirectionIOwe, PersonName: "Dilshod", Currency: CurrencyUZS, Amount: 720_000})
	closed := f.create(usd(DirectionIOwe, "Closed", 999))
	if _, err := f.service.Complete(ctx, f.owner, closed.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	summary, err := f.service.Summary(ctx, f.owner)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	want := Summary{USD: Totals{TheyOweMe: 2500, IOwe: 800}, UZS: Totals{TheyOweMe: 1_975_000, IOwe: 720_000}}
	if summary != want {
		t.Fatalf("summary = %#v, want %#v (completed debts excluded)", summary, want)
	}
}

func TestList_FiltersSearchAndPaging(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	akmal := f.create(usd(DirectionTheyOweMe, "Akmal", 1500))
	f.create(usd(DirectionIOwe, "Dilshod", 400))
	phone := f.create(CreateInput{Direction: DirectionTheyOweMe, PersonName: "Customer", PersonPhone: "+998907774422", Currency: CurrencyUSD, Amount: 1600})
	underscore := f.create(usd(DirectionIOwe, "a_b", 10))
	f.create(usd(DirectionIOwe, "axb", 10))
	if _, err := f.service.Complete(ctx, f.owner, akmal.ID); err != nil {
		t.Fatalf("complete: %v", err)
	}

	count := func(input ListInput) int {
		t.Helper()
		debts, err := f.service.List(ctx, f.owner, input)
		if err != nil {
			t.Fatalf("list %#v: %v", input, err)
		}
		return len(debts)
	}
	if count(ListInput{}) != 4 {
		t.Fatal("default list shows active debts only")
	}
	if count(ListInput{Status: "completed"}) != 1 || count(ListInput{Status: "all"}) != 5 {
		t.Fatal("status filter")
	}
	if count(ListInput{Direction: DirectionTheyOweMe}) != 1 || count(ListInput{Direction: DirectionIOwe}) != 3 {
		t.Fatal("direction filter")
	}
	if count(ListInput{Query: "dilsh"}) != 1 {
		t.Fatal("name search is case-insensitive")
	}
	if got, _ := f.service.List(ctx, f.owner, ListInput{Query: "90 777-44"}); len(got) != 1 || got[0].ID != phone.ID {
		t.Fatalf("phone search with spaces = %#v", got)
	}
	if got, _ := f.service.List(ctx, f.owner, ListInput{Query: "a_b"}); len(got) != 1 || got[0].ID != underscore.ID {
		t.Fatalf("underscore must match literally, got %#v", got)
	}
	if count(ListInput{Query: "%"}) != 0 {
		t.Fatal("percent must match literally")
	}
	page, _ := f.service.List(ctx, f.owner, ListInput{Status: "all", Limit: 2, Offset: 1})
	if len(page) != 2 {
		t.Fatalf("page size = %d, want 2", len(page))
	}
	for _, bad := range []ListInput{{Direction: "x"}, {Status: "x"}, {Query: strings.Repeat("q", 101)}, {Limit: 101}, {Offset: -1}} {
		if _, err := f.service.List(ctx, f.owner, bad); !errors.Is(err, apperror.ErrInvalidData) {
			t.Fatalf("list %#v error = %v, want invalid data", bad, err)
		}
	}
}
