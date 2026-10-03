# Debt API

Ushbu hujjat shaxsiy qarzlar endpointlarini tavsiflaydi.

## Umumiy talablar

Barcha endpointlar authentication talab qiladi:

```http
Authorization: Bearer <access_token>
Application-Language: uz | ru | en
```

Response formati va xatolar `ERRORS.md` dagi kabi.

## Qisqacha mantiq

- Qarz **shaxsiy**: uni faqat yaratgan foydalanuvchi ko‘radi. Guruh a'zolari, guruh egasi va admin ham ko‘rmaydi.
- Qarz orderlar va guruhlardan **mustaqil**: order'dan yaratilmaydi, hech qanday guruh balansiga ta'sir qilmaydi.
- Yo‘nalish: `they_owe_me` (menga qarzdor) yoki `i_owe` (men qarzdorman).
- Valyuta: `USD` yoki `UZS`, yaratilgandan keyin o‘zgarmaydi. Summalar butun son.
- Qisman to‘lov mumkin. Har bir to‘lov alohida yozuv bo‘lib qoladi va o‘zgartirilmaydi.
- Qolgan summa 0 ga tushsa, qarz avtomatik `completed` bo‘ladi.
- "Mark as Completed" qarzni istalgan vaqtda yopadi. Qolgan summa o‘zgarmaydi, ya'ni yopilgan qarzda qancha to‘lanmay qolgani ko‘rinib turadi. Yopilgan qarz qayta ochilmaydi.
- Jami summalar valyuta bo‘yicha alohida hisoblanadi, kurs bo‘yicha qo‘shilmaydi.
- Boshqa foydalanuvchining qarzi har doim `404` qaytaradi.

## Endpointlar

| Method | Endpoint | Ekran |
| --- | --- | --- |
| `GET` | `/api/v1/debts/summary` | Total summary kartalari |
| `GET` | `/api/v1/debts` | Ro‘yxat, filtr, qidiruv |
| `POST` | `/api/v1/debts` | Add Debt |
| `GET` | `/api/v1/debts/:debtID` | Debt Details |
| `POST` | `/api/v1/debts/:debtID/repayments` | Confirm Repayment |
| `GET` | `/api/v1/debts/:debtID/repayments` | History tab |
| `POST` | `/api/v1/debts/:debtID/complete` | Mark as Completed |
| `DELETE` | `/api/v1/debts/:debtID` | O‘chirish |

## Qarz obyekti

Ro‘yxat, detail, yaratish, to‘lov va yopish javoblarida bir xil:

```json
{
  "id": "debt-uuid",
  "direction": "they_owe_me",
  "person_name": "Akmal",
  "person_phone": "",
  "currency": "USD",
  "original_amount": 1500,
  "remaining_amount": 900,
  "status": "active",
  "created_at": "2026-10-01T09:00:00Z",
  "completed_at": null
}
```

- `status`: `active` yoki `completed`.
- `person_phone` kiritilmagan bo‘lsa — bo‘sh satr.

## 1. Jami summalar

```http
GET /api/v1/debts/summary
```

```json
{
  "data": {
    "usd": {"they_owe_me": 2500, "i_owe": 800},
    "uzs": {"they_owe_me": 1975000, "i_owe": 720000}
  }
}
```

Faqat `active` qarzlarning qolgan summalari qo‘shiladi.

## 2. Qarzlar ro‘yxati

```http
GET /api/v1/debts?direction=they_owe_me&status=active&query=akmal&limit=20&offset=0
```

| Parametr | Qiymatlar | Default |
| --- | --- | --- |
| `direction` | `they_owe_me`, `i_owe` | ikkalasi ham |
| `status` | `active`, `completed`, `all` | `active` |
| `query` | Ism yoki telefon bo‘yicha qidiruv, 100 belgigacha. Telefon bo‘sh joy va chiziqcha bilan yozilsa ham topiladi (`90 777-44`) | yo‘q |
| `limit` / `offset` | 1–100 / 0–10 000 | 20 / 0 |

Eng yangisi birinchi. `data` — qarz obyektlari ro‘yxati.

## 3. Qarz qo‘shish

```http
POST /api/v1/debts
```

```json
{
  "direction": "they_owe_me",
  "person_name": "Akmal",
  "person_phone": "",
  "currency": "USD",
  "amount": 1500
}
```

- `person_name` — 1–100 belgi.
- `person_phone` ixtiyoriy.
- `amount`: USD uchun 1 – 1 000 000 000, UZS uchun 1 – 1 000 000 000 000.

Response `201` — yangi qarz.

## 4. Qarz detail

```http
GET /api/v1/debts/:debtID
```

Response — qarz obyekti.

## 5. To‘lov kiritish

```http
POST /api/v1/debts/:debtID/repayments
```

```json
{
  "amount": 600
}
```

- Faqat `active` qarzga, aks holda `409`.
- `amount` 0 dan katta va qolgan summadan oshmasligi kerak, aks holda `400` (masalan: "To‘lov qolgan summadan (900 USD) oshmasligi kerak").
- Bir vaqtda kelgan ikki to‘lov qarzni manfiyga tushira olmaydi.

Response `201` — yangilangan qarz.

## 6. To‘lovlar tarixi (History tab)

```http
GET /api/v1/debts/:debtID/repayments?limit=20&offset=0
```

```json
{
  "data": [
    {"id": "repayment-uuid", "amount": 900, "created_at": "2026-10-01T12:00:00Z"},
    {"id": "repayment-uuid", "amount": 600, "created_at": "2026-10-01T10:00:00Z"}
  ]
}
```

Eng yangisi birinchi.

## 7. Qarzni yopish (Mark as Completed)

```http
POST /api/v1/debts/:debtID/complete
```

Body kerak emas. Response — yopilgan qarz. Qarz allaqachon yopilgan bo‘lsa — `409`.

## 8. Qarzni o‘chirish

```http
DELETE /api/v1/debts/:debtID
```

To‘lovlar bo‘lsa ham o‘chiriladi. Qarz ro‘yxat, jami va detaildan yo‘qoladi.

## Xatoliklar

| HTTP | Holat |
| --- | --- |
| `400` | Noto‘g‘ri yo‘nalish, valyuta yoki holat; bo‘sh yoki uzun ism; noto‘g‘ri telefon; summa chegaradan tashqarida; to‘lov 0 yoki qolgan summadan katta |
| `401` | Token yo‘q yoki yaroqsiz |
| `404` | Qarz topilmadi, o‘chirilgan yoki boshqa foydalanuvchiniki |
| `409` | Yopilgan qarzga to‘lov qilish yoki uni qayta yopish |

Har bir xatoda `message` aniq sababni so‘ralgan tilda qaytaradi (`ERRORS.md` ga qarang).

## Hozircha mavjud emas

- Qarzni tahrirlash.
