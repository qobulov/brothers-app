# Order API

Ushbu hujjat guruh ichidagi buyurtma (order) endpointlarini tavsiflaydi.

## Umumiy talablar

Barcha endpointlar authentication talab qiladi:

```http
Authorization: Bearer <access_token>
Accept-Language: uz | ru | en
```

Response formati `GROUP_API.md` dagi umumiy envelope bilan bir xil.

## Qisqacha mantiq

- Har bir orderda ikki employee bor: **giver** (mijozdan naqd pul oladi) va **receiver** (boshqa mijozga pul beradi).
- Order yaratilgandan keyin ikkala tomon alohida-alohida haqiqatda qancha pul olgani/berganini tasdiqlaydi.
- Ikkala summa teng bo‘lsa, order `completed` bo‘ladi va balanslar yangilanadi:
  - giver `balance_usd` += summa;
  - receiver `balance_usd` −= summa (balans manfiy bo‘lishi mumkin).
- Summalar farq qilsa, order `pending` holatida qoladi va `state = amount_mismatch` bo‘ladi. Tomonlardan biri o‘z tasdiqini tuzatishi kerak.
- Fee (UZS) ixtiyoriy. Har bir tomon tasdiqlashda **o‘zi olgan** fee'ni kiritadi (0 bo‘lishi mumkin). Order yakunlanganda har kimning fee'si o‘zining shu oydagi profitiga yoziladi (oy Asia/Tashkent vaqti bo‘yicha).
- Summalar butun son: USD — butun dollar, UZS — butun so‘m.

## Endpointlar

| Method | Endpoint | Ruxsat |
| --- | --- | --- |
| `POST` | `/api/v1/groups/:groupID/orders` | Owner/manager — istalgan ikki employee uchun; employee — faqat o‘zi ishtirok etgan order uchun |
| `GET` | `/api/v1/groups/:groupID/orders` | Owner, manager, investor — hammasi; employee — faqat o‘z orderlari |
| `GET` | `/api/v1/groups/:groupID/orders/:orderID` | Ro‘yxat bilan bir xil |
| `PATCH` | `/api/v1/groups/:groupID/orders/:orderID` | Yaratish qoidasi bilan bir xil, faqat `pending` order |
| `POST` | `/api/v1/groups/:groupID/orders/:orderID/confirmations` | Faqat giver yoki receiver, o‘zi uchun |
| `GET` | `/api/v1/groups/:groupID/orders/:orderID/events` | Ro‘yxat bilan bir xil |
| `POST` | `/api/v1/groups/:groupID/orders/:orderID/cancellation` | Giver yoki receiver; tasdiqlanmagan orderni yaratuvchi ham |
| `POST` | `/api/v1/groups/:groupID/orders/:orderID/cancellation/action` | Faqat giver yoki receiver |

Employee boshqa employee'larning orderini ochsa, API `404` qaytaradi.

## 1. Order yaratish

```http
POST /api/v1/groups/:groupID/orders
```

```json
{
  "giver_user_id": "user-uuid",
  "giver_customer_phone": "+998901234567",
  "receiver_user_id": "user-uuid",
  "receiver_customer_phone": "+998907654321",
  "amount_usd": 7000,
  "fee_uzs": 50000
}
```

- Giver va receiver — guruhdagi ikki xil, faol **employee**.
- Ikkala mijoz telefoni majburiy. Mijoz guruhning customers ro‘yxatidan topiladi yoki avtomatik yaratiladi.
- `fee_uzs` ixtiyoriy. U giver'ning tasdiqlash formasini oldindan to‘ldirish uchun ishlatiladi.
- Yaratuvchi avtomatik tasdiqlanmaydi.

Response `201` — order detail (3-bo‘limga qarang).

## 2. Orderlar ro‘yxati

```http
GET /api/v1/groups/:groupID/orders?status=pending&limit=50&offset=0
```

- `status`: `pending`, `completed`, `cancelled` (ixtiyoriy).
- `limit`: 1–100, default 50. `offset`: 0–10000.
- Eng yangilari birinchi.

```json
{
  "data": [
    {
      "id": "order-uuid",
      "status": "pending",
      "state": "waiting_for_you",
      "amount_usd": 7000,
      "giver": {"user_id": "uuid", "name": "Javohir", "location_name": "Tashkent"},
      "receiver": {"user_id": "uuid", "name": "Aziz", "location_name": "Kokand"},
      "created_at": "2026-09-29T10:42:00Z"
    }
  ]
}
```

Employee'ga location biriktirilmagan bo‘lsa, `location_name` qaytmaydi.

## 3. Order detail

```http
GET /api/v1/groups/:groupID/orders/:orderID
```

```json
{
  "data": {
    "id": "order-uuid",
    "group_id": "group-uuid",
    "status": "pending",
    "state": "amount_mismatch",
    "amount_usd": 7000,
    "fee_uzs": 50000,
    "giver": {
      "user_id": "uuid", "name": "Javohir", "avatar_url": "",
      "location": {"id": "uuid", "name": "Tashkent"},
      "customer_phone": "+998901234567"
    },
    "receiver": {
      "user_id": "uuid", "name": "Aziz", "avatar_url": "",
      "location": {"id": "uuid", "name": "Kokand"},
      "customer_phone": "+998907654321"
    },
    "created_by": {"user_id": "uuid", "name": "Javohir"},
    "confirmations": [
      {"role": "giver", "user_id": "uuid", "status": "confirmed", "amount_usd": 7000, "fee_uzs": 50000, "confirmed_at": "2026-09-29T10:50:00Z"},
      {"role": "receiver", "user_id": "uuid", "status": "confirmed", "amount_usd": 6950, "confirmed_at": "2026-09-29T11:05:00Z"}
    ],
    "created_at": "2026-09-29T10:42:00Z",
    "updated_at": "2026-09-29T10:42:00Z",
    "completed_at": null
  }
}
```

Location order yaratilgan paytdagi holatda saqlanadi: employee keyin boshqa joyga o‘tkazilsa ham, order tarixida eski shahar ko‘rinadi.

### `state` qiymatlari

| `state` | Ma'nosi |
| --- | --- |
| `waiting_for_you` | Siz giver yoki receiver'siz va hali tasdiqlamagansiz |
| `waiting_for_confirmation` | Boshqa tomon tasdiqlashi kutilmoqda |
| `amount_mismatch` | Ikkala tomon tasdiqladi, lekin summalar farq qiladi |
| `cancellation_requested` | Bekor qilish so‘ralgan, ikkinchi tomon javobi kutilmoqda |
| `completed` | Order yakunlangan |
| `cancelled` | Order bekor qilingan |

### Tasdiq maydonlarining ko‘rinishi

- Giver/receiver o‘z `amount_usd` va `fee_uzs`'ini doim ko‘radi.
- Qarshi tomonning `amount_usd`'i faqat o‘zingiz tasdiqlaganingizdan keyin ko‘rinadi.
- Qarshi tomonning `fee_uzs`'i giver/receiver'ga hech qachon ko‘rinmaydi.
- Owner, manager va investor hamma maydonni ko‘radi.

Yashirin maydonlar JSON'da umuman qaytmaydi.

## 4. Orderni tahrirlash

```http
PATCH /api/v1/groups/:groupID/orders/:orderID
```

```json
{
  "amount_usd": 6800,
  "fee_uzs": 40000
}
```

- Faqat yuborilgan maydonlar o‘zgaradi. Barcha maydonlar 1-bo‘limdagidek.
- Faqat `pending` order tahrirlanadi, aks holda `409`.
- Biror narsa o‘zgarsa, ikkala tasdiq o‘chadi va ikkala tomon qaytadan tasdiqlashi kerak.
- Employee o‘zini orderdan chiqarib yubora olmaydi (`403`).

## 5. Tasdiqlash yoki tasdiqni tuzatish

```http
POST /api/v1/groups/:groupID/orders/:orderID/confirmations
```

```json
{
  "amount_usd": 7000,
  "fee_uzs": 50000
}
```

- `amount_usd` — siz haqiqatda olgan/bergan summa. `fee_uzs` — siz olgan fee (0 bo‘lishi mumkin).
- Birinchi yuborish — tasdiq, keyingisi — tuzatish ("Correct My Confirmation").
- Manager va investor tasdiqlay olmaydi (`403`).
- Yakunlangan orderni tasdiqlab bo‘lmaydi (`409`).

Response — yangilangan order detail.

## 6. Order tarixi

```http
GET /api/v1/groups/:groupID/orders/:orderID/events
```

```json
{
  "data": [
    {"id": "uuid", "event_type": "created", "actor": {"user_id": "uuid", "name": "Javohir"}, "payload": {}, "created_at": "..."},
    {"id": "uuid", "event_type": "updated", "actor": {"user_id": "uuid", "name": "Javohir"}, "payload": {"changes": {"amount_usd": {"old": 7000, "new": 6800}}}, "created_at": "..."},
    {"id": "uuid", "event_type": "completed", "actor": {"user_id": "uuid", "name": "Aziz"}, "payload": {"amount_usd": 6800}, "created_at": "..."}
  ]
}
```

`event_type`: `created`, `updated`, `confirmed`, `confirmation_corrected`, `amount_mismatch`, `completed`, `cancellation_requested`, `cancellation_rejected`, `cancelled`. Eng eskisi birinchi.

Tasdiq event'larida summa va fee saqlanmaydi, shuning uchun tarix orqali qarshi tomon raqamlarini bilib bo‘lmaydi.

## 7. Orderni bekor qilish

Ikki holat bor:

- **Hech kim tasdiqlamagan `pending` order** darhol bekor qilinadi, rozilik kerak emas. Buni giver, receiver yoki orderni yaratgan odam (masalan, manager) qila oladi. Response'da `status: "cancelled"` qaytadi.
- **Kamida bitta tasdiq bor yoki order `completed`** — ikkala employee roziligi kerak. So‘rovni giver ham, receiver ham yuborishi mumkin; so‘rov yuborgan tomon avtomatik rozi hisoblanadi.

### So‘rov yuborish

```http
POST /api/v1/groups/:groupID/orders/:orderID/cancellation
```

```json
{
  "reason": "Mijoz fikridan qaytdi"
}
```

- Body ixtiyoriy, `reason` — 500 belgigacha.
- `pending` yoki `completed` orderga yuboriladi. Bekor qilingan orderga — `409`.
- Bir vaqtda faqat bitta ochiq so‘rov bo‘ladi (`409`).
- So‘rov ochiq turganda orderni tasdiqlash va tahrirlash mumkin emas (`409`).

Response `201` — order detail, ichida `cancellation`:

```json
"cancellation": {
  "id": "uuid",
  "status": "pending",
  "requested_by": {"user_id": "uuid", "name": "Javohir"},
  "reason": "Mijoz fikridan qaytdi",
  "requested_at": "2026-09-29T12:00:00Z",
  "approvals": [
    {"role": "giver", "user_id": "uuid", "status": "approved"},
    {"role": "receiver", "user_id": "uuid", "status": "waiting"}
  ]
}
```

Ochiq so‘rov bo‘lmasa, `cancellation` = `null`. Order bekor qilingandan keyin tasdiqlangan so‘rov (`status: "approved"`) qaytadi.

### Javob berish

```http
POST /api/v1/groups/:groupID/orders/:orderID/cancellation/action
```

```json
{
  "action": "approve"
}
```

- `approve` — faqat ikkinchi tomon ("Approve Cancellation"). Order `cancelled` bo‘ladi.
- `reject` — ikkala tomon ham: ikkinchi tomon uchun "Keep Order", so‘rov yuborgan uchun so‘rovni qaytarib olish.
- Order `completed` bo‘lgan bo‘lsa, bekor qilish USD balans va UZS profit ta'sirini teskari qaytaradi: giver −summa, receiver +summa, har kimning fee'si bekor qilingan oy profitidan ayriladi (profit manfiy bo‘lishi mumkin). Asl oy profiti o‘zgarmaydi.
- Bekor qilingan order tarixda qoladi.

## Bildirishnomalar

Bildirishnomalar `/api/v1/notifications` orqali olinadi. `event_type`:

| `event_type` | Kimga |
| --- | --- |
| `ORDER_CREATED` | Yaratuvchidan boshqa tomon(lar) |
| `ORDER_UPDATED` | Tahrirlovchidan boshqa tomon(lar) |
| `ORDER_AMOUNT_MISMATCH` | Ikkala tomon |
| `ORDER_COMPLETED` | Ikkala tomon |
| `ORDER_CANCELLATION_REQUESTED` | So‘rov yuborgandan boshqa tomon |
| `ORDER_CANCELLATION_REJECTED` | Rad etgandan boshqa tomon |
| `ORDER_CANCELLED` | Ikkala tomon |

Payload'da `order_id` va `group_id` bor.

## Xatoliklar

| HTTP | Holat |
| --- | --- |
| `400` | Noto‘g‘ri summa, telefon, ID yoki giver = receiver |
| `401` | Token yo‘q yoki noto‘g‘ri |
| `403` | Orderni ko‘rasiz, lekin bu amalga ruxsat yo‘q |
| `404` | Guruh a'zosi emassiz, order topilmadi yoki employee boshqaning orderini ochdi; party faol employee emas |
| `409` | `pending` bo‘lmagan orderni tahrirlash yoki tasdiqlash; ochiq bekor qilish so‘rovi bor; bekor qilish so‘rovi yo‘q |

## Hozircha mavjud emas

- Attachment'lar — 3-bosqich.
