package group

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/qobulov/brothers-app/pkg/apperror"
	"github.com/qobulov/brothers-app/pkg/database"
)

func TestService_CreateInviteAndAccept(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	ownerID := createUser(t, pool, "owner@example.com", "group-owner")
	recipientID := createUser(t, pool, "employee@example.com", "group-employee")
	_, err := pool.Exec(context.Background(), `
		UPDATE users
		SET first_name = 'Aziz', last_name = 'Karimov', avatar_url = 'https://cdn.example.com/aziz.jpg'
		WHERE id = $1
	`, recipientID)
	if err != nil {
		t.Fatalf("update employee profile: %v", err)
	}
	service := NewService(pool)

	created, err := service.Create(context.Background(), ownerID, "Tashkent ↔ Kokand")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if !created.IsOwner || created.Role != "manager" {
		t.Fatalf("owner group = %#v, want manager owner", created)
	}

	invitation, err := service.Invite(context.Background(), ownerID, created.ID, InviteInput{
		Email: "employee@example.com",
		Role:  "employee",
	})
	if err != nil {
		t.Fatalf("create invitation: %v", err)
	}
	if invitation.RecipientID != recipientID || invitation.Status != "pending" {
		t.Fatalf("invitation = %#v, want pending invitation for recipient", invitation)
	}
	var notification struct {
		ID        uuid.UUID
		Type      string `json:"type"`
		Titles    notificationTranslations
		Contents  notificationTranslations
		Payload   json.RawMessage `json:"payload"`
		Recipient uuid.UUID       `json:"recipient"`
		IsRead    bool            `json:"is_read"`
	}
	err = pool.QueryRow(context.Background(), `
		SELECT notifications.id, notifications.type::text,
		       notifications.title_en, notifications.title_uz, notifications.title_ru,
		       notifications.content_en, notifications.content_uz, notifications.content_ru,
		       notifications.payload,
		       notification_recipients.user_id, notification_recipients.is_read
		FROM notifications
		JOIN notification_recipients ON notification_recipients.notification_id = notifications.id
		WHERE notification_recipients.user_id = $1
	`, recipientID).Scan(
		&notification.ID, &notification.Type,
		&notification.Titles.English, &notification.Titles.Uzbek, &notification.Titles.Russian,
		&notification.Contents.English, &notification.Contents.Uzbek, &notification.Contents.Russian,
		&notification.Payload,
		&notification.Recipient, &notification.IsRead,
	)
	if err != nil {
		t.Fatalf("get invitation notification: %v", err)
	}
	if notification.Type != "TARGETED" || notification.Recipient != recipientID || notification.IsRead {
		t.Fatalf("notification = %#v, want unread targeted notification for recipient", notification)
	}
	if notification.Titles.English != "Group invitation" || notification.Titles.Uzbek != "Guruhga taklif" || notification.Titles.Russian != "Приглашение в группу" {
		t.Fatalf("notification titles = %#v", notification.Titles)
	}
	if notification.Contents.English != "You have been invited to join Tashkent ↔ Kokand" ||
		notification.Contents.Uzbek != "Siz Tashkent ↔ Kokand guruhiga qo'shilish uchun taklif qilindingiz" ||
		notification.Contents.Russian != "Вас пригласили присоединиться к группе Tashkent ↔ Kokand" {
		t.Fatalf("notification contents = %#v", notification.Contents)
	}
	var payload struct {
		EventType    string    `json:"event_type"`
		InvitationID uuid.UUID `json:"invitation_id"`
	}
	if err := json.Unmarshal(notification.Payload, &payload); err != nil {
		t.Fatalf("decode notification payload: %v", err)
	}
	if payload.EventType != "GROUP_INVITATION" || payload.InvitationID != invitation.ID {
		t.Fatalf("notification payload = %#v, want invitation %s", payload, invitation.ID)
	}

	pending, err := service.ListMembers(context.Background(), ownerID, created.ID, ListMembersInput{})
	if err != nil {
		t.Fatalf("list pending members: %v", err)
	}
	if len(pending) != 2 || pending[1].Status != "pending" || pending[1].InvitationID == nil || *pending[1].InvitationID != invitation.ID {
		t.Fatalf("members = %#v, want pending member invitation %s", pending, invitation.ID)
	}
	if pending[0].MemberID == nil || pending[0].AccessLevel != "overall_control" {
		t.Fatalf("owner member = %#v, want active owner with overall control", pending[0])
	}
	if pending[1].MemberID != nil || pending[1].FullName != "Aziz Karimov" || pending[1].BalanceUSD != nil || pending[1].ProfitUZS != nil {
		t.Fatalf("pending employee = %#v, want profile without financial fields", pending[1])
	}

	accepted, err := service.RespondInvitation(context.Background(), recipientID, invitation.ID, "accept")
	if err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	if accepted.Status != "accepted" || accepted.RespondedAt == nil {
		t.Fatalf("accepted invitation = %#v, want accepted with response time", accepted)
	}
	var notificationRead bool
	err = pool.QueryRow(context.Background(), `
		SELECT notification_recipients.is_read
		FROM notification_recipients
		JOIN notifications ON notifications.id = notification_recipients.notification_id
		WHERE notification_recipients.user_id = $1
		  AND notifications.payload->>'invitation_id' = $2
	`, recipientID, invitation.ID.String()).Scan(&notificationRead)
	if err != nil {
		t.Fatalf("get invitation notification read state: %v", err)
	}
	if !notificationRead {
		t.Fatal("invitation notification must be marked read after acceptance")
	}

	var employeeMemberID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		SELECT id
		FROM group_members
		WHERE group_id = $1 AND user_id = $2 AND deleted_at IS NULL
	`, created.ID, recipientID).Scan(&employeeMemberID)
	if err != nil {
		t.Fatalf("get employee membership: %v", err)
	}
	tag, err := pool.Exec(context.Background(), `
		UPDATE employee_balances
		SET balance_usd = 7200, updated_at = now()
		WHERE group_id = $1 AND member_id = $2 AND deleted_at IS NULL
	`, created.ID, employeeMemberID)
	if err != nil {
		t.Fatalf("update employee balance: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("updated employee balances = %d, want 1 seeded row", tag.RowsAffected())
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO employee_balances (group_id, member_id, balance_usd, deleted_at)
		VALUES ($1, $2, 999999, now())
	`, created.ID, employeeMemberID)
	if err != nil {
		t.Fatalf("create deleted employee balance: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO member_profit_periods (group_id, member_id, year, month, profit_uzs)
		VALUES ($1, $2, 2026, 8, 400000), ($1, $2, 2026, 9, 540000)
	`, created.ID, employeeMemberID)
	if err != nil {
		t.Fatalf("create active member profit periods: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO member_profit_periods (
			group_id, member_id, year, month, profit_uzs, deleted_at
		)
		VALUES ($1, $2, 2026, 7, 99999999, now())
	`, created.ID, employeeMemberID)
	if err != nil {
		t.Fatalf("create deleted member profit period: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO locations (group_id, name, employee_id, created_by)
		VALUES ($1, 'Tashkent', $2, $3), ($1, 'Kokand', NULL, $3)
	`, created.ID, employeeMemberID, ownerID)
	if err != nil {
		t.Fatalf("create active locations: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO locations (group_id, name, employee_id, created_by, deleted_at)
		VALUES ($1, 'Deleted location', NULL, $2, now())
	`, created.ID, ownerID)
	if err != nil {
		t.Fatalf("create deleted location: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO customers (group_id, phone)
		VALUES ($1, '+998901111111'), ($1, '+998902222222')
	`, created.ID)
	if err != nil {
		t.Fatalf("create active customers: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO customers (group_id, phone, deleted_at)
		VALUES ($1, '+998903333333', now())
	`, created.ID)
	if err != nil {
		t.Fatalf("create deleted customer: %v", err)
	}
	addGroupOrder(t, pool, created.ID, ownerID, false)
	addGroupOrder(t, pool, created.ID, ownerID, false)
	addGroupOrder(t, pool, created.ID, ownerID, true)

	groups, err := service.List(context.Background(), recipientID)
	if err != nil {
		t.Fatalf("list recipient groups: %v", err)
	}
	if len(groups) != 1 || groups[0].ID != created.ID || groups[0].Role != "employee" {
		t.Fatalf("recipient groups = %#v, want employee membership", groups)
	}
	if groups[0].GroupBalanceUSD != 7200 {
		t.Fatalf("employee group balance = %d, want 7200", groups[0].GroupBalanceUSD)
	}
	if groups[0].MyProfitUZS == nil || *groups[0].MyProfitUZS != 940000 {
		t.Fatalf("employee profit = %v, want 940000", groups[0].MyProfitUZS)
	}
	if groups[0].MembersCount != 2 || groups[0].LocationsCount != 2 ||
		groups[0].CustomersCount != 2 || groups[0].OrderCount != 2 {
		t.Fatalf("employee group counts = members:%d locations:%d customers:%d orders:%d, want 2/2/2/2",
			groups[0].MembersCount, groups[0].LocationsCount,
			groups[0].CustomersCount, groups[0].OrderCount)
	}
	if !groups[0].SubscriptionActive {
		t.Fatal("employee group subscription must use the active mock")
	}
	encodedEmployee, err := json.Marshal(groups[0])
	if err != nil {
		t.Fatalf("encode employee group: %v", err)
	}
	var employeeJSON map[string]any
	if err := json.Unmarshal(encodedEmployee, &employeeJSON); err != nil {
		t.Fatalf("decode employee group: %v", err)
	}
	if employeeJSON["group_balance_usd"] != float64(7200) || employeeJSON["my_profit_uzs"] != float64(940000) {
		t.Fatalf("employee money fields must use whole currency units: %s", encodedEmployee)
	}
	for _, unexpectedField := range []string{
		"member_id", "user_id", "full_name", "username", "avatar_url", "status", "location_name",
		"balance_usd", "profit_uzs", "orders_count", "group_balance_usd_cents", "my_profit_uzs_tiyin",
	} {
		if _, exists := employeeJSON[unexpectedField]; exists {
			t.Fatalf("group list response must not expose %s: %s", unexpectedField, encodedEmployee)
		}
	}
	if employeeJSON["order_count"] != float64(2) || employeeJSON["subscription_active"] != true {
		t.Fatalf("group list summary fields = %s", encodedEmployee)
	}

	ownerGroups, err := service.List(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list owner groups: %v", err)
	}
	if len(ownerGroups) != 1 || ownerGroups[0].GroupBalanceUSD != 7200 {
		t.Fatalf("owner groups = %#v, want shared group balance", ownerGroups)
	}
	if ownerGroups[0].MyProfitUZS == nil || *ownerGroups[0].MyProfitUZS != 940000 {
		t.Fatalf("owner profit = %v, want group total 940000", ownerGroups[0].MyProfitUZS)
	}
	encodedOwner, err := json.Marshal(ownerGroups[0])
	if err != nil {
		t.Fatalf("encode owner group: %v", err)
	}
	var ownerJSON map[string]any
	if err := json.Unmarshal(encodedOwner, &ownerJSON); err != nil {
		t.Fatalf("decode owner group: %v", err)
	}
	if ownerJSON["my_profit_uzs"] != float64(940000) {
		t.Fatalf("owner response my_profit_uzs = %v, want 940000: %s", ownerJSON["my_profit_uzs"], encodedOwner)
	}

	if ownerGroups[0].MembersCount != 2 || ownerGroups[0].LocationsCount != 2 ||
		ownerGroups[0].CustomersCount != 2 || ownerGroups[0].OrderCount != 2 ||
		!ownerGroups[0].SubscriptionActive {
		t.Fatalf("owner group list item = %#v", ownerGroups[0])
	}

	members, err := service.ListMembers(context.Background(), ownerID, created.ID, ListMembersInput{
		Query:  "Tashkent",
		Role:   "employee",
		Status: "active",
	})
	if err != nil {
		t.Fatalf("list filtered members: %v", err)
	}
	if len(members) != 1 {
		t.Fatalf("filtered members = %#v, want one employee", members)
	}
	employee := members[0]
	if employee.MemberID == nil || *employee.MemberID != employeeMemberID || employee.UserID != recipientID {
		t.Fatalf("employee identity = %#v", employee)
	}
	if employee.FullName != "Aziz Karimov" || employee.AvatarURL != "https://cdn.example.com/aziz.jpg" || employee.AccessLevel != "assigned" {
		t.Fatalf("employee profile = %#v", employee)
	}
	if employee.LocationName == nil || *employee.LocationName != "Tashkent" {
		t.Fatalf("employee location = %v, want Tashkent", employee.LocationName)
	}
	if employee.BalanceUSD == nil || *employee.BalanceUSD != 7200 || employee.ProfitUZS == nil || *employee.ProfitUZS != 940000 {
		t.Fatalf("employee financial data = balance:%v profit:%v, want 7200/940000", employee.BalanceUSD, employee.ProfitUZS)
	}

	managers, err := service.ListMembers(context.Background(), ownerID, created.ID, ListMembersInput{Role: "manager"})
	if err != nil {
		t.Fatalf("list managers: %v", err)
	}
	if len(managers) != 1 || !managers[0].IsOwner || managers[0].BalanceUSD != nil || managers[0].ProfitUZS != nil {
		t.Fatalf("manager rows = %#v, want owner without employee financial fields", managers)
	}

	locations, err := service.ListLocations(context.Background(), recipientID, created.ID)
	if err != nil {
		t.Fatalf("list locations as group member: %v", err)
	}
	var tashkent, kokand Location
	for _, location := range locations {
		switch location.Name {
		case "Tashkent":
			tashkent = location
		case "Kokand":
			kokand = location
		}
	}
	if len(locations) != 2 || tashkent.ID == uuid.Nil || tashkent.Employee == nil || tashkent.Employee.ID != recipientID || kokand.ID == uuid.Nil {
		t.Fatalf("locations = %#v, want assigned Tashkent and unassigned Kokand", locations)
	}
	customers, err := service.ListCustomers(context.Background(), recipientID, created.ID, "2222")
	if err != nil {
		t.Fatalf("search customers as group member: %v", err)
	}
	if len(customers) != 1 || customers[0].Phone != "+998902222222" {
		t.Fatalf("customers = %#v, want filtered active customer", customers)
	}

	fergana, err := service.CreateLocation(context.Background(), ownerID, created.ID, CreateLocationInput{Name: " Fergana "})
	if err != nil {
		t.Fatalf("create location: %v", err)
	}
	if fergana.Name != "Fergana" || fergana.Employee != nil {
		t.Fatalf("created location = %#v", fergana)
	}
	if err := service.DeleteLocation(context.Background(), ownerID, created.ID, tashkent.ID); !errors.Is(err, apperror.ErrConflict) {
		t.Fatalf("delete assigned location: error = %v, want conflict", err)
	}
	if err := service.DeleteLocation(context.Background(), ownerID, created.ID, fergana.ID); err != nil {
		t.Fatalf("delete unassigned location: %v", err)
	}
	if _, err := service.CreateLocation(context.Background(), recipientID, created.ID, CreateLocationInput{Name: "Bukhara"}); !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("create location as employee: error = %v, want forbidden", err)
	}

	outsiderID := createUser(t, pool, "outsider@example.com", "group-outsider")
	if _, err := service.ListCustomers(context.Background(), outsiderID, created.ID, ""); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("list customers as outsider: error = %v, want record not found", err)
	}
}

func TestService_Delete(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	ownerID := createUser(t, pool, "delete-owner@example.com", "delete-owner")
	managerID := createUser(t, pool, "delete-manager@example.com", "delete-manager")
	employeeID := createUser(t, pool, "delete-employee@example.com", "delete-employee")
	inviteeID := createUser(t, pool, "delete-invitee@example.com", "delete-invitee")
	service := NewService(pool)

	created, err := service.Create(context.Background(), ownerID, "Delete me")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	addGroupMember(t, pool, created.ID, managerID, "manager")
	employeeMemberID := addGroupMember(t, pool, created.ID, employeeID, "employee")
	addEmployeeBalance(t, pool, created.ID, employeeMemberID, 7200)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO member_profit_periods (group_id, member_id, year, month, profit_uzs)
		VALUES ($1, $2, 2026, 9, 940000)
	`, created.ID, employeeMemberID)
	if err != nil {
		t.Fatalf("create employee profit: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO locations (group_id, name, employee_id, created_by)
		VALUES ($1, 'Tashkent', $2, $3)
	`, created.ID, employeeMemberID, ownerID)
	if err != nil {
		t.Fatalf("create group location: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO customers (group_id, phone)
		VALUES ($1, '+998901234567')
	`, created.ID)
	if err != nil {
		t.Fatalf("create group customer: %v", err)
	}
	addGroupOrder(t, pool, created.ID, ownerID, false)
	invitation, err := service.Invite(context.Background(), ownerID, created.ID, InviteInput{
		UserID: inviteeID,
		Role:   "investor",
	})
	if err != nil {
		t.Fatalf("create pending invitation: %v", err)
	}

	if err := service.Delete(context.Background(), ownerID, created.ID, false); !errors.Is(err, apperror.ErrInvalidData) {
		t.Fatalf("delete without confirmation: error = %v, want invalid data", err)
	}
	if err := service.Delete(context.Background(), managerID, created.ID, true); !errors.Is(err, apperror.ErrForbidden) {
		t.Fatalf("delete as manager: error = %v, want forbidden", err)
	}
	if err := service.Delete(context.Background(), ownerID, created.ID, true); err != nil {
		t.Fatalf("delete group as owner: %v", err)
	}

	var groupActive bool
	var groupDeleted bool
	err = pool.QueryRow(context.Background(), `
		SELECT is_active, deleted_at IS NOT NULL
		FROM groups
		WHERE id = $1
	`, created.ID).Scan(&groupActive, &groupDeleted)
	if err != nil {
		t.Fatalf("get deleted group state: %v", err)
	}
	if groupActive || !groupDeleted {
		t.Fatalf("group state = active:%t deleted:%t, want false/true", groupActive, groupDeleted)
	}

	for _, check := range []struct {
		name  string
		query string
		want  int
	}{
		{name: "members", query: `SELECT count(*) FROM group_members WHERE group_id = $1 AND deleted_at IS NOT NULL`, want: 3},
		{name: "locations", query: `SELECT count(*) FROM locations WHERE group_id = $1 AND deleted_at IS NOT NULL`, want: 1},
		{name: "customers", query: `SELECT count(*) FROM customers WHERE group_id = $1 AND deleted_at IS NOT NULL`, want: 1},
		{name: "orders", query: `SELECT count(*) FROM orders WHERE group_id = $1 AND deleted_at IS NOT NULL`, want: 1},
		{name: "balances preserved", query: `SELECT count(*) FROM employee_balances WHERE group_id = $1 AND deleted_at IS NULL`, want: 1},
		{name: "profits preserved", query: `SELECT count(*) FROM member_profit_periods WHERE group_id = $1 AND deleted_at IS NULL`, want: 1},
	} {
		var got int
		if err := pool.QueryRow(context.Background(), check.query, created.ID).Scan(&got); err != nil {
			t.Fatalf("check %s: %v", check.name, err)
		}
		if got != check.want {
			t.Fatalf("%s count = %d, want %d", check.name, got, check.want)
		}
	}

	var invitationStatus string
	var invitationDeleted bool
	err = pool.QueryRow(context.Background(), `
		SELECT status, deleted_at IS NOT NULL
		FROM group_invitations
		WHERE id = $1
	`, invitation.ID).Scan(&invitationStatus, &invitationDeleted)
	if err != nil {
		t.Fatalf("get deleted invitation state: %v", err)
	}
	if invitationStatus != "revoked" || !invitationDeleted {
		t.Fatalf("invitation state = %s/%t, want revoked/deleted", invitationStatus, invitationDeleted)
	}

	var notificationRead bool
	err = pool.QueryRow(context.Background(), `
		SELECT recipients.is_read
		FROM notification_recipients recipients
		JOIN notifications ON notifications.id = recipients.notification_id
		WHERE notifications.payload->>'invitation_id' = $1
	`, invitation.ID.String()).Scan(&notificationRead)
	if err != nil {
		t.Fatalf("get invitation notification state: %v", err)
	}
	if !notificationRead {
		t.Fatal("pending invitation notification must be read after group deletion")
	}

	var auditActor uuid.UUID
	var auditAction, auditEntityType string
	var auditEntityID uuid.UUID
	err = pool.QueryRow(context.Background(), `
		SELECT actor_user_id, action, entity_type, entity_id
		FROM audit_logs
		WHERE group_id = $1 AND deleted_at IS NULL
	`, created.ID).Scan(&auditActor, &auditAction, &auditEntityType, &auditEntityID)
	if err != nil {
		t.Fatalf("get group deletion audit log: %v", err)
	}
	if auditActor != ownerID || auditAction != "group.deleted" || auditEntityType != "group" || auditEntityID != created.ID {
		t.Fatalf("audit log = %s/%s/%s/%s", auditActor, auditAction, auditEntityType, auditEntityID)
	}

	groups, err := service.List(context.Background(), ownerID)
	if err != nil {
		t.Fatalf("list groups after deletion: %v", err)
	}
	if len(groups) != 0 {
		t.Fatalf("groups after deletion = %#v, want empty", groups)
	}
	if err := service.Delete(context.Background(), ownerID, created.ID, true); !errors.Is(err, apperror.ErrRecordNotFound) {
		t.Fatalf("delete group twice: error = %v, want record not found", err)
	}
}

func TestService_GroupBalanceVisibility(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	ownerID := createUser(t, pool, "balance-owner@example.com", "balance-owner")
	managerID := createUser(t, pool, "balance-manager@example.com", "balance-manager")
	investorID := createUser(t, pool, "balance-investor@example.com", "balance-investor")
	legacyMemberID := createUser(t, pool, "balance-member@example.com", "balance-member")
	employeeAID := createUser(t, pool, "balance-employee-a@example.com", "balance-employee-a")
	employeeBID := createUser(t, pool, "balance-employee-b@example.com", "balance-employee-b")
	service := NewService(pool)

	created, err := service.Create(context.Background(), ownerID, "Balance visibility")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	addGroupMember(t, pool, created.ID, managerID, "manager")
	addGroupMember(t, pool, created.ID, investorID, "investor")
	addGroupMember(t, pool, created.ID, legacyMemberID, "member")
	employeeAMemberID := addGroupMember(t, pool, created.ID, employeeAID, "employee")
	employeeBMemberID := addGroupMember(t, pool, created.ID, employeeBID, "employee")
	addEmployeeBalance(t, pool, created.ID, employeeAMemberID, 7200)
	addEmployeeBalance(t, pool, created.ID, employeeBMemberID, 3100)
	_, err = pool.Exec(context.Background(), `
		INSERT INTO member_profit_periods (group_id, member_id, year, month, profit_uzs)
		VALUES ($1, $2, 2026, 9, 500000), ($1, $2, 2026, 10, -100000), ($1, $3, 2026, 10, 250000)
	`, created.ID, employeeAMemberID, employeeBMemberID)
	if err != nil {
		t.Fatalf("create employee profits: %v", err)
	}

	tests := []struct {
		name        string
		actorID     uuid.UUID
		wantBalance int64
		wantProfit  *int64
	}{
		{name: "owner sees group totals", actorID: ownerID, wantBalance: 10300, wantProfit: profitUZS(650000)},
		{name: "manager sees group totals", actorID: managerID, wantBalance: 10300, wantProfit: profitUZS(650000)},
		{name: "investor sees read-only group totals", actorID: investorID, wantBalance: 10300, wantProfit: profitUZS(650000)},
		{name: "legacy member sees no totals", actorID: legacyMemberID, wantBalance: 0},
		{name: "first employee sees own totals", actorID: employeeAID, wantBalance: 7200, wantProfit: profitUZS(400000)},
		{name: "second employee sees own totals", actorID: employeeBID, wantBalance: 3100, wantProfit: profitUZS(250000)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			groups, err := service.List(context.Background(), tt.actorID)
			if err != nil {
				t.Fatalf("list groups: %v", err)
			}
			if len(groups) != 1 || groups[0].GroupBalanceUSD != tt.wantBalance {
				t.Fatalf("list balance = %#v, want %d", groups, tt.wantBalance)
			}
			got := groups[0].MyProfitUZS
			if (got == nil) != (tt.wantProfit == nil) || (got != nil && *got != *tt.wantProfit) {
				t.Fatalf("list profit = %v, want %v", got, tt.wantProfit)
			}

		})
	}

	investors, err := service.ListMembers(context.Background(), ownerID, created.ID, ListMembersInput{Role: "investor"})
	if err != nil {
		t.Fatalf("list investors: %v", err)
	}
	if len(investors) != 1 || investors[0].AccessLevel != "read_only" || investors[0].BalanceUSD != nil || investors[0].ProfitUZS != nil {
		t.Fatalf("investor rows = %#v, want read-only without employee financial fields", investors)
	}
}

func TestService_InviteAfterExpiredInvitation(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	ownerID := createUser(t, pool, "expiry-owner@example.com", "expiry-owner")
	recipientID := createUser(t, pool, "expiry-recipient@example.com", "expiry-recipient")
	service := NewService(pool)
	created, err := service.Create(context.Background(), ownerID, "Expiry")
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	input := InviteInput{UserID: recipientID, Role: "employee"}
	first, err := service.Invite(context.Background(), ownerID, created.ID, input)
	if err != nil {
		t.Fatalf("first invitation: %v", err)
	}
	if _, err := service.Invite(context.Background(), ownerID, created.ID, input); !errors.Is(apperror.Normalize(err), apperror.ErrDuplicatedKey) {
		t.Fatalf("duplicate pending invitation error = %v, want duplicated key", err)
	}

	_, err = pool.Exec(context.Background(), `UPDATE group_invitations SET expires_at = now() - interval '1 minute' WHERE id = $1`, first.ID)
	if err != nil {
		t.Fatalf("expire invitation: %v", err)
	}
	second, err := service.Invite(context.Background(), ownerID, created.ID, input)
	if err != nil {
		t.Fatalf("invitation after expiry: %v", err)
	}
	if second.ID == first.ID {
		t.Fatal("expected a new invitation")
	}
	var firstStatus string
	if err := pool.QueryRow(context.Background(), `SELECT status FROM group_invitations WHERE id = $1`, first.ID).Scan(&firstStatus); err != nil {
		t.Fatalf("get expired invitation: %v", err)
	}
	if firstStatus != "revoked" {
		t.Fatalf("expired invitation status = %q, want revoked", firstStatus)
	}

	if _, err := service.RespondInvitation(context.Background(), recipientID, second.ID, "accept"); err != nil {
		t.Fatalf("accept invitation: %v", err)
	}
	if _, err := service.Invite(context.Background(), ownerID, created.ID, input); !errors.Is(err, apperror.ErrAlreadyExists) {
		t.Fatalf("invite existing member error = %v, want already exists", err)
	}
}

func TestService_CreateRejectsDeletedActor(t *testing.T) {
	pool, cleanup := database.SetupTestDB(t)
	defer cleanup()

	actorID := createUser(t, pool, "deleted-actor@example.com", "deleted-actor")
	if _, err := pool.Exec(context.Background(), `UPDATE users SET deleted_at = now() WHERE id = $1`, actorID); err != nil {
		t.Fatalf("delete actor: %v", err)
	}
	if _, err := NewService(pool).Create(context.Background(), actorID, "Orphan"); !errors.Is(err, apperror.ErrUnauthorized) {
		t.Fatalf("create error = %v, want unauthorized", err)
	}
	var groups int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM groups`).Scan(&groups); err != nil {
		t.Fatalf("count groups: %v", err)
	}
	if groups != 0 {
		t.Fatalf("groups = %d, want no ownerless group", groups)
	}
}

func TestValidMemberFilters(t *testing.T) {
	tests := []struct {
		name       string
		input      ListMembersInput
		wantQuery  string
		wantRole   string
		wantStatus string
		wantError  bool
	}{
		{name: "empty", input: ListMembersInput{}},
		{name: "normalize all", input: ListMembersInput{Query: " Aziz ", Role: "ALL", Status: " all "}, wantQuery: "Aziz"},
		{name: "employee active", input: ListMembersInput{Role: "Employee", Status: "ACTIVE"}, wantRole: "employee", wantStatus: "active"},
		{name: "invalid role", input: ListMembersInput{Role: "owner"}, wantError: true},
		{name: "invalid status", input: ListMembersInput{Status: "deleted"}, wantError: true},
		{name: "query too long", input: ListMembersInput{Query: strings.Repeat("x", 101)}, wantError: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query, role, status, err := validMemberFilters(tt.input)
			if tt.wantError {
				if !errors.Is(err, apperror.ErrInvalidData) {
					t.Fatalf("error = %v, want invalid data", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("valid filters: %v", err)
			}
			if query != tt.wantQuery || role != tt.wantRole || status != tt.wantStatus {
				t.Fatalf("filters = %q/%q/%q, want %q/%q/%q", query, role, status, tt.wantQuery, tt.wantRole, tt.wantStatus)
			}
		})
	}
}

func TestGroupInvitationNotificationTranslations(t *testing.T) {
	t.Parallel()

	titles, contents := groupInvitationNotificationTranslations("Oilam")
	if titles.English != "Group invitation" || titles.Uzbek != "Guruhga taklif" || titles.Russian != "Приглашение в группу" {
		t.Fatalf("titles = %#v", titles)
	}
	if contents.English != "You have been invited to join Oilam" ||
		contents.Uzbek != "Siz Oilam guruhiga qo'shilish uchun taklif qilindingiz" ||
		contents.Russian != "Вас пригласили присоединиться к группе Oilam" {
		t.Fatalf("contents = %#v", contents)
	}
}

func createUser(t *testing.T, pool *pgxpool.Pool, email, username string) uuid.UUID {
	t.Helper()
	userID := uuid.New()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO users (id, email, username, language, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, 'uz', true, now(), now())
	`, userID, email, username)
	if err != nil {
		t.Fatalf("create test user: %v", err)
	}
	return userID
}

func addGroupMember(t *testing.T, pool *pgxpool.Pool, groupID, userID uuid.UUID, role string) uuid.UUID {
	t.Helper()
	var memberID uuid.UUID
	err := pool.QueryRow(context.Background(), `
		INSERT INTO group_members (group_id, user_id, username, role)
		SELECT $1, id, username, $3::user_role
		FROM users
		WHERE id = $2
		RETURNING id
	`, groupID, userID, role).Scan(&memberID)
	if err != nil {
		t.Fatalf("add %s group member: %v", role, err)
	}
	return memberID
}

func addEmployeeBalance(t *testing.T, pool *pgxpool.Pool, groupID, memberID uuid.UUID, balance int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO employee_balances (group_id, member_id, balance_usd)
		VALUES ($1, $2, $3)
	`, groupID, memberID, balance)
	if err != nil {
		t.Fatalf("add employee balance: %v", err)
	}
}

// addGroupOrder inserts an order between the group's first two members using
// an existing active customer, so member and customer counts stay unchanged.
func addGroupOrder(t *testing.T, pool *pgxpool.Pool, groupID, createdBy uuid.UUID, deleted bool) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		WITH members AS (
			SELECT id, row_number() OVER (ORDER BY joined_at, id) AS position
			FROM group_members
			WHERE group_id = $1 AND deleted_at IS NULL
		), customer AS (
			SELECT id FROM customers WHERE group_id = $1 AND deleted_at IS NULL ORDER BY id LIMIT 1
		)
		INSERT INTO orders (
			group_id, created_by, giver_member_id, receiver_member_id,
			giver_customer_id, receiver_customer_id, amount_usd, deleted_at
		)
		SELECT $1, $2,
		       (SELECT id FROM members WHERE position = 1),
		       (SELECT id FROM members WHERE position = 2),
		       customer.id, customer.id, 7000,
		       CASE WHEN $3 THEN now() END
		FROM customer
	`, groupID, createdBy, deleted)
	if err != nil {
		t.Fatalf("create group order: %v", err)
	}
}

func profitUZS(value int64) *int64 { return &value }
