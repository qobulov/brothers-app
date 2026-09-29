# Group API

Ushbu hujjat group bilan bog‘liq amaldagi REST API endpointlarini tavsiflaydi.

## Umumiy talablar

Barcha endpointlar authentication talab qiladi:

```http
Authorization: Bearer <access_token>
Accept-Language: uz | ru | en
```

`Accept-Language` berilmasa, response xabarlari default English tilida qaytadi.

## Umumiy response formati

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Localized message",
  "data": {},
  "meta": {
    "timestamp": "2026-09-29T10:00:00Z",
    "request_id": "...",
    "api_version": "v1",
    "duration": "1.2ms"
  }
}
```

## Endpointlar

| Method | Endpoint | Ruxsat |
| --- | --- | --- |
| `POST` | `/api/v1/groups` | Authenticated user |
| `GET` | `/api/v1/groups` | Authenticated user |
| `DELETE` | `/api/v1/groups/:groupID` | Faqat owner |
| `POST` | `/api/v1/groups/:groupID/invitations` | Owner/manager |
| `POST` | `/api/v1/invitations/:invitationID/action` | Taklif qilingan user |
| `GET` | `/api/v1/groups/:groupID/members` | Owner/manager |
| `GET` | `/api/v1/groups/:groupID/locations` | Group member |
| `POST` | `/api/v1/groups/:groupID/locations` | Owner/manager |
| `DELETE` | `/api/v1/groups/:groupID/locations/:locationID` | Owner/manager |
| `GET` | `/api/v1/groups/:groupID/customers` | Group member |

## 1. Group yaratish

```http
POST /api/v1/groups
```

Request:

```json
{
  "name": "Tashkent ↔ Kokand"
}
```

Response — `201 Created`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Group created",
  "data": {
    "id": "group-uuid",
    "name": "Tashkent ↔ Kokand",
    "role": "manager",
    "is_owner": true
  },
  "meta": {}
}
```

Group yaratgan user avtomatik `manager` va `is_owner: true` bo‘ladi.

## 2. Mening grouplarim

```http
GET /api/v1/groups
```

Request body yo‘q. `data` har doim array bo‘ladi.

Response — `200 OK`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Groups returned",
  "data": [
    {
      "id": "group-uuid",
      "name": "Tashkent ↔ Kokand",
      "role": "employee",
      "is_owner": false,
      "group_balance_usd": 7200,
      "my_profit_uzs": 940000,
      "members_count": 6,
      "locations_count": 4,
      "customers_count": 15,
      "order_count": 222,
      "subscription_active": true
    }
  ],
  "meta": {}
}
```

Qoidalar:

- Employee `group_balance_usd`da faqat o‘z balansini ko‘radi.
- Owner, manager va investor umumiy group balansini ko‘radi.
- `my_profit_uzs` faqat employee uchun qaytadi.
- `subscription_active` hozircha doim mock `true`.
- Pul qiymatlari cent yoki tiyin emas.

## 3. Group o‘chirish

```http
DELETE /api/v1/groups/:groupID
```

Faqat `is_owner: true` user ishlata oladi.

Request:

```json
{
  "confirm": true
}
```

Response — `200 OK`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Group deleted",
  "data": null,
  "meta": {}
}
```

Bu soft-delete: moliyaviy va audit tarixi saqlanadi.

## 4. Groupga user taklif qilish

```http
POST /api/v1/groups/:groupID/invitations
```

Faqat owner yoki manager ishlata oladi. `user_id` yoki `email`dan faqat bittasi yuborilishi kerak.

Request, user ID orqali:

```json
{
  "user_id": "user-uuid",
  "role": "employee",
  "location_name": "Kokand"
}
```

Request, email orqali:

```json
{
  "email": "aziz@example.com",
  "role": "employee",
  "location_name": "Kokand"
}
```

Ruxsat etilgan role qiymatlari:

```text
employee | manager | investor
```

Response — `201 Created`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Invitation created",
  "data": {
    "id": "invitation-uuid",
    "group_id": "group-uuid",
    "group_name": "Tashkent ↔ Kokand",
    "invited_by": "manager-user-uuid",
    "recipient_id": "recipient-user-uuid",
    "email": "aziz@example.com",
    "role": "employee",
    "location_name": "Kokand",
    "status": "pending",
    "expires_at": "2026-10-06T10:00:00Z"
  },
  "meta": {}
}
```

Taklif bilan birga English, Uzbek va Russian tillarida notification yaratiladi.

## 5. Taklifni qabul qilish yoki rad etish

```http
POST /api/v1/invitations/:invitationID/action
```

Request:

```json
{
  "action": "accept"
}
```

Ruxsat etilgan qiymatlar:

```text
accept | reject
```

Response — `200 OK`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Invitation action processed",
  "data": {
    "id": "invitation-uuid",
    "group_id": "group-uuid",
    "group_name": "Tashkent ↔ Kokand",
    "invited_by": "manager-user-uuid",
    "recipient_id": "recipient-user-uuid",
    "email": "aziz@example.com",
    "role": "employee",
    "status": "accepted",
    "expires_at": "2026-10-06T10:00:00Z",
    "responded_at": "2026-09-29T10:10:00Z"
  },
  "meta": {}
}
```

Employee taklifni qabul qilsa, uning boshlang‘ich balansi `0 USD` bilan yaratiladi.

## 6. Group members

```http
GET /api/v1/groups/:groupID/members
```

Faqat owner yoki manager ishlata oladi.

Query parametrlar:

```text
query=Aziz
role=all|manager|employee|investor
status=all|active|pending
```

Misol:

```http
GET /api/v1/groups/:groupID/members?query=Aziz&role=employee&status=active
```

Response — `200 OK`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Members returned",
  "data": [
    {
      "member_id": "member-uuid",
      "user_id": "user-uuid",
      "full_name": "Aziz Karimov",
      "username": "aziz",
      "email": "aziz@example.com",
      "avatar_url": "https://cdn.example.com/aziz.jpg",
      "role": "employee",
      "status": "active",
      "is_owner": false,
      "access_level": "assigned",
      "location_name": "Kokand",
      "balance_usd": 7200,
      "profit_uzs": 940000
    }
  ],
  "meta": {}
}
```

`access_level` qiymatlari:

```text
overall_control | manage | assigned | read_only
```

Pending user uchun:

- `member_id` qaytmaydi.
- `invitation_id` qaytadi.
- `balance_usd` va `profit_uzs` qaytmaydi.

## 7. Group locations

```http
GET /api/v1/groups/:groupID/locations
```

Groupdagi istalgan active member ishlata oladi.

Response — `200 OK`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Request processed successfully",
  "data": [
    {
      "id": "location-uuid",
      "group_id": "group-uuid",
      "name": "Kokand",
      "employee": {
        "id": "employee-user-uuid",
        "name": "Aziz Karimov",
        "avatar_url": "https://cdn.example.com/aziz.jpg"
      },
      "created_by": "manager-user-uuid"
    }
  ],
  "meta": {}
}
```

Locationga employee biriktirilmagan bo‘lsa, `employee` fieldi qaytmaydi.

## 8. Location yaratish

```http
POST /api/v1/groups/:groupID/locations
```

Faqat owner yoki manager ishlata oladi.

Request:

```json
{
  "name": "Kokand",
  "employee_id": "employee-user-uuid"
}
```

`employee_id` optional va `user_id`ni bildiradi, `member_id`ni emas. User shu groupdagi active employee bo‘lishi kerak.

Response — `201 Created`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Request processed successfully",
  "data": {
    "id": "location-uuid",
    "group_id": "group-uuid",
    "name": "Kokand",
    "employee": {
      "id": "employee-user-uuid",
      "name": "Aziz Karimov",
      "avatar_url": "https://cdn.example.com/aziz.jpg"
    },
    "created_by": "manager-user-uuid"
  },
  "meta": {}
}
```

## 9. Location o‘chirish

```http
DELETE /api/v1/groups/:groupID/locations/:locationID
```

Faqat owner yoki manager ishlata oladi. Locationga employee biriktirilgan bo‘lsa, API `409 Conflict` qaytaradi.

Response — `200 OK`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Request processed successfully",
  "data": null,
  "meta": {}
}
```

## 10. Group customers

```http
GET /api/v1/groups/:groupID/customers
```

Optional phone qidiruvi:

```http
GET /api/v1/groups/:groupID/customers?query=99890
```

`query` maksimal 20 ta belgidan iborat bo‘lishi mumkin.

Response — `200 OK`:

```json
{
  "success": true,
  "code": 0,
  "slug": "ok",
  "message": "Request processed successfully",
  "data": [
    {
      "id": "customer-uuid",
      "phone": "+998901234567"
    }
  ],
  "meta": {}
}
```

Bu hozircha read-only API. Customer create, update va delete endpointlari mavjud emas.

## Error response

```json
{
  "success": false,
  "code": 1403,
  "slug": "forbidden",
  "message": "Forbidden",
  "data": {
    "reason": "forbidden"
  },
  "meta": {
    "timestamp": "2026-09-29T10:00:00Z",
    "request_id": "...",
    "api_version": "v1",
    "duration": "1.2ms"
  }
}
```

Asosiy status kodlari:

| HTTP status | Ma’nosi |
| --- | --- |
| `400` | Request body, UUID yoki filter noto‘g‘ri |
| `401` | Authentication yo‘q yoki session yaroqsiz |
| `403` | Ushbu amal uchun permission yetarli emas |
| `404` | Group, user, invitation yoki location topilmadi |
| `409` | Duplicate, allaqachon member, expired invitation yoki biriktirilgan location konflikti |

## Hozir mavjud bo‘lmagan API’lar

- `GET /api/v1/groups/:groupID`
- Group update yoki rename
- Member update yoki delete
- Employee balance boshqarish
- Profit boshqarish
- Investor boshqaruvi
- Customer create, update yoki delete
- Subscription API
- List pagination
