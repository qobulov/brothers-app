# Xatoliklar bilan ishlash

Ushbu hujjat mobil ilova API xatolarini qanday ko‘rsatishi va qayta ishlashi kerakligini tavsiflaydi.

## Har bir so‘rovda

```http
Application-Language: uz | ru | en
```

Til shu headerdan olinadi. U bo‘lmasa, `Accept-Language` ishlatiladi, ikkalasi ham bo‘lmasa — English.

## Xato javobi

Har qanday xatoda javob bir xil shaklda keladi:

```json
{
  "success": false,
  "code": 1400,
  "slug": "invalid_data",
  "message": "Summa 1 dan 1 000 000 000 dollargacha bo'lishi kerak",
  "data": {
    "reason": "invalid data: The amount must be between $1 and $1,000,000,000"
  },
  "meta": {
    "timestamp": "2026-09-30T18:00:00Z",
    "request_id": "872ab4678c89fe13107f993ce00ff384",
    "api_version": "v1",
    "duration": "1.2ms"
  }
}
```

| Maydon | Nima uchun | Ilova nima qiladi |
| --- | --- | --- |
| `message` | Foydalanuvchi uchun matn, so‘rov tilida. Aniq sabab bo‘lsa, shu sabab | **Foydalanuvchiga shuni ko‘rsating** |
| `slug` | Xato turi, tilga bog‘liq emas | **Ilova mantiqi uchun shuni ishlating** |
| `code` | Raqamli xato kodi (`1000 + HTTP status`) | `slug` o‘rniga ishlatish mumkin |
| `data.reason` | Dasturchilar uchun texnik izoh, doim English | Foydalanuvchiga ko‘rsatmang, faqat log uchun |
| `meta.request_id` | So‘rov ID'si | Xatoda logga yozing: shu ID bo‘yicha server logidan so‘rov topiladi |

Qoidalar:

- `data` xatoda hech qachon `null` bo‘lmaydi, ichida doim `reason` bor.
- `message` matnini kodda tekshirmang (`if message == ...`): u tilga qarab o‘zgaradi. Mantiq uchun faqat `slug` yoki `code`.
- Server xatolarida (5xx) production'da `data.reason` faqat slug'ni qaytaradi, masalan `"internal_error"`. Texnik tafsilot faqat server logida.

## Ilova mantiqi

```
javob keldi, success == false:
    foydalanuvchiga message ni ko'rsatish
    slug bo'yicha qo'shimcha amal (quyidagi jadval)
    meta.request_id va data.reason ni logga yozish

javob kelmadi (internet yo'q, timeout):
    ilovaning o'z matni: "Internet aloqasi yo'q" / "Server javob bermadi"
    qayta urinish tugmasi
```

## `slug` jadvali

| `slug` | `code` | HTTP | Ilova nima qiladi |
| --- | --- | --- | --- |
| `invalid_data` | 1400 | 400 | `message` ni forma yonida yoki toast'da ko‘rsatish |
| `invalid_id` | 1400 | 400 | `message` ko‘rsatish (odatda ilova xatosi) |
| `invalid_or_expired_otp` | 1400 | 400 | OTP maydonini tozalash, `message` ko‘rsatish |
| `unauthorized` | 1401 | 401 | Avval `POST /auth/refresh`. U ham 401 bo‘lsa — login sahifasi |
| `invalid_credentials` | 1401 | 401 | Login formasida `message` |
| `session_revoked` | 1401 | 401 | Login sahifasi: boshqa qurilmadan kirilgan yoki chiqilgan |
| `invalid_or_expired_reset_token` | 1401 | 401 | Parol tiklashni boshidan boshlash |
| `forbidden` | 1403 | 403 | `message` ko‘rsatish. Odatda UI bunday tugmani ko‘rsatmasligi kerak (`permissions` maydonlariga qarang) |
| `not_found` | 1404 | 404 | "Topilmadi" holati yoki oldingi sahifaga qaytish |
| `conflict` | 1409 | 409 | `message` ko‘rsatish va ma'lumotni qayta yuklash: server tomonda holat o‘zgargan |
| `email_or_username_exists` | 1409 | 409 | Ro‘yxatdan o‘tish formasida `message` |
| `rate_limit_exceeded` | 1429 | 429 | `message` ko‘rsatish, "qayta yuborish" tugmasini kutish vaqtiga qadar o‘chirish |

`429` qaytadigan holatlar (bitta IP manzil bo‘yicha hisoblanadi):

| Endpoint | Chegara |
| --- | --- |
| `POST /auth/login` | bitta login uchun 15 daqiqada 10 ta urinish; jami 15 daqiqada 100 ta |
| `GET /auth/username/check` | daqiqasiga 60 ta |
| `POST /auth/otp/send` | 15 daqiqada 20 ta, hamda bitta email uchun qayta yuborish kutish vaqti |

Username tekshiruvini har bir harfda emas, yozish to‘xtagach (300–500 ms debounce) yuboring.
| `email_delivery_unavailable` | 1503 | 503 | "Keyinroq urinib ko‘ring" |
| `timeout` | 1504 | 504 | Qayta urinish tugmasi |
| `internal_error` | 1500 | 500 | Umumiy xato matni, `meta.request_id` ni logga yozish |

`conflict` misollari: order allaqachon yakunlangan, bekor qilish so‘rovi allaqachon ochiq, a'zoning balansi 0 emas, joy boshqa xodimga biriktirilgan. Har birida `message` aniq sababni aytadi.
