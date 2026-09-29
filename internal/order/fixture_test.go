package order

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/database"
)

// fixture is one group with an owner, a manager, an investor and three
// employees. Javohir (Tashkent) and Aziz (Kokand) are the usual parties;
// the bystander is an employee with no location who is not on the order.
type fixture struct {
	t       *testing.T
	pool    *pgxpool.Pool
	service *Service

	groupID                            uuid.UUID
	owner, manager, investor, stranger uuid.UUID
	giver, receiver, bystander         uuid.UUID
	giverMember, receiverMember        uuid.UUID
	tashkentID, kokandID               uuid.UUID
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	pool, cleanup := database.SetupTestDB(t)
	t.Cleanup(cleanup)
	f := &fixture{t: t, pool: pool, service: NewService(pool)}
	f.service.now = steppingClock(time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC))

	f.owner = f.user("owner")
	f.manager = f.user("manager")
	f.investor = f.user("investor")
	f.giver = f.user("javohir")
	f.receiver = f.user("aziz")
	f.bystander = f.user("bystander")
	f.stranger = f.user("stranger")

	f.groupID = f.queryID(`INSERT INTO groups (name, created_by) VALUES ('Brothers', $1) RETURNING id`, f.owner)
	f.member(f.owner, "manager", true)
	f.member(f.manager, "manager", false)
	f.member(f.investor, "investor", false)
	f.giverMember = f.member(f.giver, "employee", false)
	f.receiverMember = f.member(f.receiver, "employee", false)
	f.member(f.bystander, "employee", false)
	f.tashkentID = f.location("Tashkent", f.giverMember)
	f.kokandID = f.location("Kokand", f.receiverMember)
	return f
}

// steppingClock returns strictly increasing times so newest-first ordering is
// stable within a test. It is safe for concurrent use.
func steppingClock(start time.Time) func() time.Time {
	var mu sync.Mutex
	current := start
	return func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		current = current.Add(time.Second)
		return current
	}
}

func (f *fixture) exec(sql string, args ...any) {
	f.t.Helper()
	if _, err := f.pool.Exec(context.Background(), sql, args...); err != nil {
		f.t.Fatalf("exec %q: %v", sql, err)
	}
}

func (f *fixture) queryID(sql string, args ...any) uuid.UUID {
	f.t.Helper()
	var id uuid.UUID
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&id); err != nil {
		f.t.Fatalf("query %q: %v", sql, err)
	}
	return id
}

func (f *fixture) count(sql string, args ...any) int {
	f.t.Helper()
	var n int
	if err := f.pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		f.t.Fatalf("count %q: %v", sql, err)
	}
	return n
}

func (f *fixture) user(username string) uuid.UUID {
	return f.queryID(`
		INSERT INTO users (email, username, first_name, language, is_active)
		VALUES ($1, $2, $2, 'uz', true)
		RETURNING id
	`, username+"@example.com", username)
}

func (f *fixture) member(userID uuid.UUID, role string, isOwner bool) uuid.UUID {
	return f.queryID(`
		INSERT INTO group_members (group_id, user_id, username, role, is_owner)
		SELECT $1, id, username, $3::user_role, $4 FROM users WHERE id = $2
		RETURNING id
	`, f.groupID, userID, role, isOwner)
}

func (f *fixture) location(name string, memberID uuid.UUID) uuid.UUID {
	return f.queryID(`
		INSERT INTO locations (group_id, name, employee_id, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, f.groupID, name, memberID, f.owner)
}

func (f *fixture) createInput() CreateInput {
	return CreateInput{
		GiverUserID:           f.giver,
		GiverCustomerPhone:    "+998 90 111 11 11",
		ReceiverUserID:        f.receiver,
		ReceiverCustomerPhone: "+998 90 222 22 22",
		AmountUSD:             7000,
		FeeUZS:                50000,
	}
}

func (f *fixture) create(actorID uuid.UUID) Order {
	f.t.Helper()
	created, err := f.service.Create(context.Background(), actorID, f.groupID, f.createInput())
	if err != nil {
		f.t.Fatalf("create order: %v", err)
	}
	return created
}

func (f *fixture) notificationCount(userID uuid.UUID, eventType string) int {
	return f.count(`
		SELECT COUNT(*)
		FROM notification_recipients recipients
		JOIN notifications ON notifications.id = recipients.notification_id
		WHERE recipients.user_id = $1 AND notifications.payload->>'event_type' = $2
	`, userID, eventType)
}

func (f *fixture) eventCount(orderID uuid.UUID, eventType string) int {
	return f.count(`SELECT COUNT(*) FROM order_events WHERE order_id = $1 AND event_type = $2`, orderID, eventType)
}
